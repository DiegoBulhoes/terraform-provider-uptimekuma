data "uptimekuma_tag" "environment" {
  name = "environment"
}

resource "uptimekuma_monitor_http" "web" {
  name = "Web"
  url  = "https://example.com"

  tags = [
    {
      tag_id = data.uptimekuma_tag.environment.id
      value  = "production"
    },
  ]
}
