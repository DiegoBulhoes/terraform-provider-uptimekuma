resource "uptimekuma_monitor_json_query" "health" {
  name               = "Health endpoint reports ready"
  url                = "https://api.example.com/health"
  json_path          = "status"
  json_path_operator = "=="
  expected_value     = "ready"
}
