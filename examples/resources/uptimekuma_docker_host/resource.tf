resource "uptimekuma_docker_host" "local" {
  name            = "Local daemon"
  connection_type = "socket"
  daemon          = "/var/run/docker.sock"
}

resource "uptimekuma_docker_host" "remote" {
  name            = "Build server"
  connection_type = "tcp"
  daemon          = "tcp://build.internal:2375"
}
