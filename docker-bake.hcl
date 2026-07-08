variable "CI" {
  default = false
}

target "docker-metadata-action" {}

target "_common" {
  context = "."
  dockerfile = "Dockerfile.CI"
}

target "image-all" {
  inherits = ["_common"]
  matrix = {
    mod = [
      {name = "cni", bin = "loom"},
      {name = "operator", bin = "manager"},
      {name = "daemon", bin = "daemon"}
    ]
  }
  target = "runtime"
  name = "image-${mod.name}"
  args = {
    MOD = mod.name
    BIN = mod.bin
  }
  cache-from = ["type=gha,scope=${mod.name}"]
  cache-to   = ["type=gha,scope=${mod.name},mode=max"]
  tags = CI ? [
    for tag in target.docker-metadata-action.tags : replace(tag, "__service__", "${mod.name}")
  ] : ["${mod.name}:local"]
}

target "test-all" {
  inherits = ["_common"]
  matrix = {
    mod = [
      "cni",
      "operator",
      "daemon"
    ]
  }
  target = "test"
  name = "test-${mod}"
  args = {
    MOD = mod
  }
  output = [
    "type=local,dest=artifacts"
  ]
}
