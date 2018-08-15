*Background*

**Goal**

Dynamically controlling pod resource limit

**Setup**

Suppose you have a Kubernetes cluster for testing. You need to dynamically change a service resource usage like CPU, memory, network bandwidth to verify if the service still function correctly.

Implement a kubectl plugin that can dynamically changing a pod’s resource limit.

```
kubectl plugin chaos --set-cpu=500m -n <ns> <pod-name> -c <container-name>
kubectl plugin chaos --set-memory=2Gi -n <ns> <pod-name> -c <container-name>
kubectl plugin chaos --set-bandwidth=2Mbps -n <ns> <pod-name> -c <container-name>
```

You need to implement an agent and deploy it on every worker node, so the plugin can communicate with it to control the running container’s resource.

Note: You can’t recreate the pod or container to apply resources limit. You must do it at runtime.

*Design*

**Initial Thoughts**

When first reading this exercise, my mind immediately went to the `docker update` (command)[https://docs.docker.com/engine/reference/commandline/update/. `docker update` will allow for modifying both CPU and memory resources of the pod's running containers at runtime with the following commands:

```
docker update --cpus=".5" <container>
docker update --mememory="2g" <container>
```

These commands would allocate half of a CPU core, and 2 GB of memory to the running container without having to restart it. Kubernetes itself doesn't support running native docker commands on pod containers, and doesn't currently have support to wrap the `docker update` command for `kubectl`, hence the need for a plugin and agent setup.

While `docker update` does support dynamically allocating CPU and memory resources, it does not support limiting the network bandwidth of a running container. In order to do this, we will have to leverage `docker exec` in conjunction with Linux traffic control (`tc`). The docker command will end up looking like:

```
tc qdisc add dev eth0 root tbf rate 2mbps
```

Note that because this command requires that the container has permission to modify its own network interface, the capability will need to be added to each container in the kubernetes deployment as shown below:

```
apiVersion: v1
kind: Pod
metadata:
 name: hello-minikube-net-admin
spec:
 containers:
   - name: hello-minikube
     image: "k8s.gcr.io/echoserver:1.10"
     securityContext:
       capabilities:
         add:
           - NET_ADMIN
```

**Implementation**

The implementation will be divided into two parts: the agent, and the `kubectl` plugin. I have opted to write both in Go as it is what I am most comfortable with.

In order for the agent to run the native docker commands needed on the containers in the kubernetes cluster, it will need to be running on the node machines themselves. With this in mind, in order to communicate with the agent via the plugin, a simple HTTP interface makes the most sense to receive commands. The agent will serve a single POST endpoint with the specified as part of the URL. The arguments will be parsed, validated, and the correct command will be executed. Once finished running the command, the output of the command will written to the response body of the request so the caller knows exactly what happened. Only returned exit code of 0 from the command will result in a 200 response.

Due to the design of the agent, the plugin will be relatively simple. The plugin will need to identify the node on which the pod is running, and POST the command and its arguments to the agent's HTTP endpoint. The plugin should also display information on whether the command was successful, and if not, show the output of the command for debugging purposes.

*Usage*

First we need to compile the two binaries:

```
go build agent/realloc-agent.go
go build plugin/realloc.go
```

The agent should then be run on each of the cluster nodes like so:

```
$ ./realloc-agent
running realloc agent on port: 8000...
```

The `kubectl` plugin should then be installed by setting the following environment variable:

```
KUBECTL_PLUGINS_PATH=$GOPATH/github.com/pc-rocket/pck8s/
```

**CPU**

```
$ kubectl plugin realloc --set-cpu=500m -p hello-minikube-7c77b68cff-zcjnj -c hello-minikube
{"output":"9685b0241d9fc578468fbc2e0f1e0a425807b811a491bae9708c3af3ed91c4e1\n"}
```

**Memory**

```
$ kubectl plugin realloc --set-memory=2Gi -p hello-minikube-7c77b68cff-zcjnj -c hello-minikube
{"output":"9685b0241d9fc578468fbc2e0f1e0a425807b811a491bae9708c3af3ed91c4e1\n"}
```

**Network**

Because this required the container to have the `NET_ADMIN` capability enabled, a separate pod was created with the configuration mentioned earlier.

```
$ kubectl plugin realloc --set-bandwidth=2Mbps -p hello-minikube-net-admin -c hello-minikube
{"output":""}

```