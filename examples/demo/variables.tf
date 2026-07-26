variable "endpoint" {
  description = "Base URL of the demo Uptime Kuma instance."
  type        = string
  default     = "http://localhost:3001"
}

variable "username" {
  description = "Admin username created by `make up`."
  type        = string
  default     = "demo"
}

variable "password" {
  description = "Admin password created by `make up`."
  type        = string
  default     = "demo123"
  sensitive   = true
}

variable "status_page_slug" {
  description = "Slug for the demo status page."
  type        = string
  default     = "demo"
}
