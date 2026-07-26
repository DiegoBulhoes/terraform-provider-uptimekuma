data "uptimekuma_status_pages" "all" {}

output "slugs" {
  value = [for page in data.uptimekuma_status_pages.all.status_pages : page.slug]
}
