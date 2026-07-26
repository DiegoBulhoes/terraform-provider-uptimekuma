resource "uptimekuma_tag" "environment" {
  name  = "environment"
  color = "#4B5563"
}

resource "uptimekuma_monitor_http" "web" {
  name = "Web"
  url  = "https://example.com"

  tags = [
    {
      tag_id = uptimekuma_tag.environment.id
      value  = "production"
    },
  ]
}
