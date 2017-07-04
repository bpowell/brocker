# brocker
Containers and orchestration without using big boy tools

## Overview
There are two main things that brocker does. First is the concept of services and the other is containers. A service manages a collection of containers. A container is just a command that is ran inside of its own PID, network and mount namespace.

### Services
A service has a bridge network interface on the host that is used to communicate with all containers inside of a service. A service is actually a nginx container that runs an nginx configuration to do a reverse proxy to multiple Spring Boot application containers. A container must have a service to tie it self to.o

### Containers
Containers are a simple command that is executed within its own PID, network, and mount namespace.

## brockerd
This is the daemon that sits in the background and listens on port 3000 for connections from the brocker-client. This process will spawn all new containers with their own PID, network, and mount namespaces. Each container will have their own network interface (veth1) connected to a vethX on the host. Each service will have their own network bridge on the host, named brockerX. Before the container starts, the container is given a name that is a sha1 hash on the timestamp and the command being run. This name is used to identify the container and gives the container a place to store anything that it needs. A directory under /container named the container's name is created that is used to store all things related to that container. That directory will be mounted to /app for the container.

## brocker-client
This is the client interface to brockerd.

### Commands:
1. container
  1. run filename.json - Runs command specified in filename.json
  2. exec container_hash command... - Runs specified command in container
  3. list - Lists all running containers
  4. stop container_hash - Stops container
2. service
  1. add filename.json - Creates a service with details from filename.json

## brocker-run
The init process for the container. This mounts the /app directory for the container to write its files to.
