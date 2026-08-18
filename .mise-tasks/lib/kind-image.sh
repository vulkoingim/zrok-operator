# shellcheck shell=bash
# Helpers for loading images into Kind. Source this file.

kind_cluster_name() {
	echo "${KIND_CLUSTER:-kind}"
}

kind_control_plane_container() {
	echo "$(kind_cluster_name)-control-plane"
}

# linux GOARCH for the local Kind node (arm64 on Apple Silicon, amd64 on GHA).
kind_target_goarch() {
	local arch="${E2E_GOARCH:-}"
	if [[ -z "$arch" ]] && command -v docker >/dev/null 2>&1; then
		arch="$(docker version -f '{{.Server.Arch}}' 2>/dev/null || true)"
	fi
	if [[ -z "$arch" ]]; then
		arch="$(go env GOARCH)"
	fi
	case "$arch" in
	aarch64) echo arm64 ;;
	*) echo "$arch" ;;
	esac
}

# Short image ID (12 hex) for crictl matching.
local_image_short_id() {
	docker image inspect -f '{{.Id}}' "$1" | sed 's/^sha256://' | cut -c1-12
}

# Return 0 when the local image ID is already present on the Kind node.
# Match ID only — same tag with a new digest must be re-imported (IfNotPresent).
kind_has_image() {
	local image="$1"
	local cluster="${2:-$(kind_cluster_name)}"
	local node="${cluster}-control-plane"

	docker image inspect "$image" >/dev/null 2>&1 || return 1
	docker inspect "$node" >/dev/null 2>&1 || return 1

	local short_id
	short_id="$(local_image_short_id "$image")"
	docker exec "$node" crictl images -q 2>/dev/null | grep -Fq "$short_id"
}

# Load image into Kind unless the node already has this image ID.
kind_load_if_needed() {
	local image="$1"
	local cluster="${2:-$(kind_cluster_name)}"

	if kind_has_image "$image" "$cluster"; then
		echo "Image $image already present on kind/$cluster; skipping load"
		return 0
	fi

	echo "Loading $image into kind/$cluster"
	if kind load docker-image "$image" --name "$cluster"; then
		return 0
	fi

	echo "kind load docker-image failed; falling back to image-archive" >&2
	local tmp
	tmp="$(mktemp)"
	trap 'rm -f "$tmp"' RETURN
	docker save -o "$tmp" "$image"
	kind load image-archive "$tmp" --name "$cluster"
}
