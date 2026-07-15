variable "CI" {
  default = false
}

#variable "GITHUB_EVENT_NAME" {
#  default = ""
#}

variable "MODS" {
  type = list(object({
    name = string
    bin  = string
  }))
  default = [
      {name = "cni", bin = "loom"},
      {name = "operator", bin = "manager"},
      {name = "daemon", bin = "daemon"}
    ]
}

target "docker-metadata-action" {
  tags = ["__target__:local"]
}

target "_common" {
  matrix = {
    mod = MODS
  }
  context = "."
  dockerfile = "Dockerfile"
  name = "_common-${mod.name}"
  args = {
    MOD = mod.name
    BIN = mod.bin
  }
  #cache-from = ["type=gha,scope=${mod.name}"]
  #cache-to = [
  #  GITHUB_EVENT_NAME == "pull_request"
  #    ? ""
  #    : "type=gha,scope=${mod.name},mode=max"
  #]
  cache-from = ["type=gha,scope=${mod.name}"]
  cache-to   = ["type=gha,scope=${mod.name},mode=max"]
}

target "image-all" {
  matrix = {
    mod = MODS
  }
  inherits = ["_common-${mod.name}", "docker-metadata-action"]
  target = "runtime"
  name = "image-${mod.name}"
  tags = [for tag in target.docker-metadata-action.tags : replace(tag, "__target__", "${mod.name}")]
}

target "test-all" {
  matrix = {
    mod = MODS
  }
  inherits = ["_common-${mod.name}"]
  target = "test"
  name = "test-${mod.name}"
  output = [
    "type=local,dest=artifacts"
  ]
}
