variable "tag_ids" {
  description = "Tag IDs to attach, keyed by name."
  type        = map(string)
  default     = {}
}

variable "notification_ids" {
  description = "Notification channel IDs, keyed by name."
  type        = map(string)
  default     = {}
}

variable "docker_host_id" {
  description = "Docker host the container monitor should query."
  type        = string
}
