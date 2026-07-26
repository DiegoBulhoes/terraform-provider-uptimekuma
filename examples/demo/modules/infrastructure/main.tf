# Things monitors depend on, plus the API key. None of these depend on anything
# else, so they can be created in parallel with tags and notifications.

resource "uptimekuma_proxy" "egress" {
  protocol = "http"
  host     = "proxy.example.com"
  port     = 3128
  active   = true
  default  = false
}

resource "uptimekuma_docker_host" "local" {
  name            = "Local daemon"
  connection_type = "socket"
  daemon          = "/var/run/docker.sock"
}

resource "uptimekuma_remote_browser" "chrome" {
  name = "Shared Chrome"
  url  = "ws://chrome.example.com:3000"
}

# API keys authenticate the Prometheus /metrics endpoint, not the API this
# provider uses. The clear-text key is returned once, at creation.
resource "uptimekuma_api_key" "prometheus" {
  name = "Prometheus scraper"
}
