resource "uptimekuma_docker_host" "local" {
  name            = "Local daemon"
  connection_type = "socket"
  daemon          = "/var/run/docker.sock"
}

resource "uptimekuma_monitor_docker" "redis" {
  name           = "Redis container"
  container_name = "redis"
  docker_host_id = uptimekuma_docker_host.local.id
}
