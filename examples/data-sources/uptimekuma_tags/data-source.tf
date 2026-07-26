data "uptimekuma_tags" "all" {}

output "tag_names" {
  value = [for tag in data.uptimekuma_tags.all.tags : tag.name]
}
