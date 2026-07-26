data "uptimekuma_docker_hosts" "all" {}

output "docker_host_names" {
  value = [for host in data.uptimekuma_docker_hosts.all.docker_hosts : host.name]
}
