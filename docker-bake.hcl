variable "CI" {
  default = false
}

# ignore-error: a gha cache upload failure (e.g. rate limiting) must not fail the build.
# PR runs export too, so later pushes to the same PR start warm; PR-scoped entries
# are deleted when the PR closes (.github/workflows/cache-cleanup.yml).
function "cache_to" {
  params = [scope]
  result = ["type=gha,scope=${scope},mode=max,ignore-error=true"]
}

variable "GOVERSION" { }

variable "MODS" {
  type = list(object({
    name = string
    bin  = string
  }))
  default = [
      {name = "cni", bin = "loom"},
      {name = "operator", bin = "manager"},
      {name = "daemon", bin = "daemon"},
      {name = "dpcs", bin = "dpcs"},
      {name = "targets-bmv2", bin = "driver"}
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
    MODPATH = replace(mod.name, "-", "/")
  }
}

target "image-all" {
  matrix = {
    mod = MODS
  }
  inherits = ["_common-${mod.name}", "docker-metadata-action"]
  target = "runtime"
  name = "image-${mod.name}"
  tags = [for tag in target.docker-metadata-action.tags : replace(tag, "__target__", "${mod.name}")]
  # Tests and images use separate scopes: a gha cache export replaces the
  # scope's index, so sharing one scope made each build erase the other's cache.
  cache-from = ["type=gha,scope=image-${mod.name}"]
  cache-to   = cache_to("image-${mod.name}")
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
  cache-from = ["type=gha,scope=test-${mod.name}"]
  cache-to   = cache_to("test-${mod.name}")
}
