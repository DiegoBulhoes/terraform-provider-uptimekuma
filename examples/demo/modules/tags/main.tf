# Tags are shared across the whole instance, so they have no dependencies and
# come first in the graph.

resource "uptimekuma_tag" "environment" {
  name  = "environment"
  color = "#4B5563"
}

resource "uptimekuma_tag" "team" {
  name  = "team"
  color = "#059669"
}
