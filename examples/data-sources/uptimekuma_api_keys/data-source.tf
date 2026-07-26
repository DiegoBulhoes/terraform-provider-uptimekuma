data "uptimekuma_api_keys" "all" {}

output "expired_api_keys" {
  value = [for key in data.uptimekuma_api_keys.all.api_keys : key.name if key.status == "expired"]
}
