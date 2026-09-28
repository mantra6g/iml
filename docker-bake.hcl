variable "CI" {
  default = false
}

variable "GITHUB_EVENT_NAME" {
  default = ""
}

# Only main/tag builds export to the gha cache: PR caches are scoped to the PR
# ref (unusable by other branches) and would evict main's entries from the quota.
function "cache_to" {
  params = [scope]
  result = GITHUB_EVENT_NAME == "pull_request" ? [] : ["type=gha,scope=${scope},mode=max,ignore-error=true"]
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
