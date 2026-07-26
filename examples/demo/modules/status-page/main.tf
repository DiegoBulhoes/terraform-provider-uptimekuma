# The whole group tree is declared here, and it is a save-everything call: a
# group left out of the configuration is deleted.
#
# Order matters twice over. Uptime Kuma derives each group's weight from its
# position, and each monitor's weight from its position inside the group, so
# `group` and `monitor` are ordered blocks rather than sets.

resource "uptimekuma_status_page" "public" {
  slug  = var.slug
  title = "Demo Status"

  description = "Everything this demo monitors"
  theme       = "dark"
  show_tags   = true
  footer_text = "Managed by Terraform"

  group {
    name = "Runs without internet"

    # send_url makes the name a link. With it on and no url of its own, the
    # server reports the monitor's own URL back.
    monitor {
      monitor_id = var.linkable_monitor_id
      send_url   = true
    }

    dynamic "monitor" {
      for_each = [for id in var.offline_safe_monitor_ids : id if id != var.linkable_monitor_id]

      content {
        monitor_id = monitor.value
      }
    }
  }

  group {
    name = "Needs outbound access"

    dynamic "monitor" {
      for_each = var.outbound_monitor_ids

      content {
        monitor_id = monitor.value
      }
    }
  }

  group {
    name = "Waiting for a heartbeat"

    monitor {
      monitor_id = var.push_monitor_id
    }
  }
}

# A page shows one incident at a time: posting pins it and unpins the previous
# one. Setting `pinned = false` resolves it instead of deleting it.
resource "uptimekuma_status_page_incident" "notice" {
  status_page_slug = uptimekuma_status_page.public.slug
  title            = "This is a demo"
  content          = "Every monitor and alert here was created by Terraform. The credentials on the alerts are fake."
  style            = "info"
}
