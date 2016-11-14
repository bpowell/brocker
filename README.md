#brocker
Containers and orchestration without using big boy tools

##brockerd
This is the daemon that sits in the background and listens on port 3000 for connections from the brocker-client. This process will spawn all new containers with their own PID, network, and mount namespaces. Each container will have their own network interface (veth1) connected to a vethX on the host. Each service will have their own network bridge on the host, named brockerX. Before the container starts, the container is given a name that is a sha1 hash on the timestamp and the command being run. This name is used to identify the container and gives the container a place to store anything that it needs. A directory under /container named the container's name is created that is used to store all things related to that container. That directory will be mounted to /app for the container.

##brocker-client
This is the client interface to brockerd.

###Commands:
1. container
  1. run filename.json - Runs command specified in filename.json
  2. exec container_hash command... - Runs specified command in container
  3. list - Lists all running containers
  4. rm container_hash - Stops container
2. service
  1. add filename.json - Creates a service with details from filename.json
