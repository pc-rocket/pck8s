package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var (
	port = "8000"
)

func init() {
	p := os.Getenv("AGENT_PORT")
	if p != "" {
		port = p
	}
}

func main() {
	fmt.Printf("running realloc agent on port: %v...\n", port)
	http.HandleFunc("/", post)
	http.ListenAndServe(":"+port, http.DefaultServeMux)
}

func post(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	command, err := parse(r.URL)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cmd := exec.Command("docker", command.Args()...)

	fmt.Println(command.Args())

	out, err := cmd.Output()

	if err != nil {
		fmt.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println(string(out))

	respond(w, map[string]interface{}{"output": string(out)})
}

func respond(w http.ResponseWriter, v interface{}) {
	// ignore error for simplicity
	w.WriteHeader(http.StatusOK)
	buf, _ := json.Marshal(v)
	w.Write(buf)
}

type Resource string

const (
	CPU       Resource = "cpu"
	Memory    Resource = "memory"
	Bandwidth Resource = "bandwidth"
)

type ResourceCommand struct {
	Resource  Resource
	Value     int64
	Units     string
	Container string
}

func (cmd *ResourceCommand) Args() (args []string) {
	switch cmd.Resource {
	case CPU:
		args = []string{
			"update",
			cmd.Container,
			fmt.Sprintf("--cpu-shares=%d%s", cmd.Value, cmd.Units),
		}
	case Memory:
		args = []string{
			"update",
			cmd.Container,
			fmt.Sprintf("--memory=%d%s", cmd.Value, cmd.Units),
		}
	case Bandwidth:
		args = []string{
			"exec",
			cmd.Container,
			fmt.Sprintf(`sh -c "tc qdisc add dev eth0 root tbf rate %d%s latency 50ms burst %d%s"`,
				cmd.Value, cmd.Units, cmd.Value, strings.TrimRight(cmd.Units, "bps")),
		}
	}

	return
}

func (cmd *ResourceCommand) ParseBandwidth(bw string) (err error) {
	var val, units []rune

	for _, r := range bw {
		switch {
		case r >= 'A' && r <= 'Z':
			fallthrough
		case r >= 'a' && r <= 'z':
			units = append(units, r)
		case r >= '0' && r <= '9':
			val = append(val, r)
		}
	}

	cmd.Value, err = strconv.ParseInt(string(val), 10, 64)

	if err != nil {
		return
	}

	u := strings.ToLower(string(units))

	fmt.Println(u)

	if u != "kbps" && u != "mbps" && u != "gbps" {
		return fmt.Errorf("invalid bandwidth units")
	}

	cmd.Units = u

	return
}

func (cmd *ResourceCommand) ParseCPU(cpu string) (err error) {
	if !strings.Contains(cpu, "m") {
		return fmt.Errorf("cpu must be specified in millicores")
	}

	parts := strings.Split(cpu, "m")
	if len(parts) < 1 {
		return fmt.Errorf("invalid cpus specified (%v)", parts)
	}

	v, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return
	}

	cmd.Value = cpuForDocker(v)

	return
}

func (cmd *ResourceCommand) ParseMemory(mem string) (err error) {
	var val, units []rune

	for _, r := range mem {
		switch {
		case r >= 'A' && r <= 'Z':
			fallthrough
		case r >= 'a' && r <= 'z':
			units = append(units, r)
		case r >= '0' && r <= '9':
			val = append(val, r)
		}
	}

	cmd.Value, err = strconv.ParseInt(string(val), 10, 64)

	if err != nil {
		return
	}

	u := strings.ToUpper(strings.Replace(string(units), "i", "", 1))

	if len(u) > 1 {
		return fmt.Errorf("invalid memory format")
	}

	cmd.Units = u

	return
}

func parse(u *url.URL) (*ResourceCommand, error) {
	q := u.Query()

	res := q.Get("resource")

	if res == "" {
		return nil, fmt.Errorf("resource not specified")
	}

	value := q.Get("value")

	if value == "" {
		return nil, fmt.Errorf("no set value specified")
	}

	container := q.Get("container")

	if container == "" {
		return nil, fmt.Errorf("no container specified")
	}

	cmd := &ResourceCommand{
		Resource:  Resource(res),
		Container: container,
	}

	var err error

	switch cmd.Resource {
	case CPU:
		err = cmd.ParseCPU(value)
	case Memory:
		err = cmd.ParseMemory(value)
	case Bandwidth:
		err = cmd.ParseBandwidth(value)
	default:
		return nil, fmt.Errorf("unsupported resource (%v)", res)
	}

	if err != nil {
		return nil, err
	}

	return cmd, nil
}

const (
	minShares     = 2
	sharesPerCPU  = 1024
	milliCPUToCPU = 1000
)

func cpuForDocker(milliCPU int64) int64 {
	if milliCPU == 0 {
		return minShares
	}

	shares := (milliCPU * sharesPerCPU) / milliCPUToCPU

	if shares < minShares {
		return minShares
	}

	return shares
}
