data "uptimekuma_proxies" "all" {}

output "default_proxy_id" {
  value = one([for proxy in data.uptimekuma_proxies.all.proxies : proxy.id if proxy.default])
}
