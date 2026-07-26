# The clear-text key is only ever returned when the key is created, so an
# imported api_key has a null `key` attribute and cannot be recovered.
terraform import uptimekuma_api_key.example 1
