#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/wait.h>
#include <unistd.h>
                     
static char child_stack[1048576];
                      
static int child_fn() {
  // calling unshare() from inside the init process lets you create a new namespace after a new process has been spawned
  unshare(CLONE_NEWNET);

  printf("New `net` Namespace:\n");
  system("ip link");
  system("ifconfig lo up");
  system("ifconfig veth1 10.0.4.15");
  system("ifconfig -a");

  system("/usr/sbin/nginx -c /home/yup/container/nginx1.conf");
  printf("\n\n");
  return 0;
}

static int java1() {
  unshare(CLONE_NEWNET);
  printf("java1: New `net` Namespace:\n");
  system("ip link");
  system("ifconfig lo up");
  system("ifconfig veth1 10.0.4.16");
  system("ifconfig -a");

  system("/usr/sbin/nginx -c /home/yup/container/nginx2.conf");
  printf("\n\n");
  return 0;
}

int main() {
  pid_t child_pid = clone(child_fn, child_stack+1048576, CLONE_NEWPID |  SIGCHLD, NULL);
  pid_t java_pid = clone(java1, child_stack+1048576, CLONE_NEWPID |  SIGCHLD, NULL);

  printf("nginx: %d\tjava: %d\n\n", child_pid, java_pid);

  char *cmd = malloc(sizeof(char) * 1024);
  sprintf(cmd, "ip link add name veth0 type veth peer name veth1 netns %d", child_pid);
  system(cmd);
  free(cmd);

  char *cmd2 = malloc(sizeof(char) * 1024);
  sprintf(cmd2, "ip link add name veth2 type veth peer name veth1 netns %d", java_pid);
  system(cmd2);
  free(cmd2);

  system("ip link add name brocker type bridge");
  system("ip link set brocker up");
  system("ifconfig brocker 10.1.0.1");
  system("ifconfig veth0 up");
  system("ifconfig veth2 up");
  system("ip link set veth0 master brocker");
  system("ip link set veth2 master brocker");

  waitpid(child_pid, NULL, 0);
  waitpid(java_pid, NULL, 0);
  return 0;
}  
