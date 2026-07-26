variable "monitor_ids" {
  description = "Monitors covered by the recurring window."
  type        = list(string)
}

variable "external_monitor_id" {
  description = "Monitor covered by the one-off window."
  type        = string
}

variable "status_page_ids" {
  description = "Status pages that should show the recurring window."
  type        = list(number)
  default     = []
}
