resource "uptimekuma_monitor_http" "web" {
  name = "Web"
  url  = "https://example.com"
}

resource "uptimekuma_monitor_http" "api" {
  name = "API"
  url  = "https://api.example.com/health"
}

resource "uptimekuma_status_page" "public" {
  slug  = "public"
  title = "Example Status"

  description = "Live status of our services"
  theme       = "auto"
  show_tags   = true
  footer_text = "Questions? support@example.com"

  # Group order is the display order, and so is monitor order inside a group.
  group {
    name = "Customer facing"

    monitor {
      monitor_id = uptimekuma_monitor_http.web.id
      send_url   = true
    }
  }

  group {
    name = "Internal"

    monitor {
      monitor_id = uptimekuma_monitor_http.api.id
    }
  }
}

output "status_page_url" {
  value = "https://kuma.example.com/status/${uptimekuma_status_page.public.slug}"
}
