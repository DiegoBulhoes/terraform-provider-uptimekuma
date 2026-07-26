variable "slug" {
  description = "URL slug for the page, so it is served at /status/<slug>."
  type        = string
  default     = "demo"
}

variable "offline_safe_monitor_ids" {
  description = "Monitors that work without internet access, shown in their own group."
  type        = list(string)
}

variable "outbound_monitor_ids" {
  description = "Monitors that need outbound access, shown in their own group."
  type        = list(string)
}

variable "push_monitor_id" {
  description = "The push monitor, which stays pending until something calls it."
  type        = string
}

variable "linkable_monitor_id" {
  description = "Monitor whose name should be a clickable link, to show send_url."
  type        = string
}
