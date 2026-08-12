#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$ROOT"

credential_pattern='192\.168\.[0-9]+\.[0-9]+|pass''word[[:space:]]*[:=]|1234''5678|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY'
if rg -n --hidden --glob '!dist/**' --glob '!.git/**' "$credential_pattern" .; then
	echo "potential credential or private environment data found" >&2
	exit 1
fi

if rg -n --glob '*.go' --glob '*.sh' \
	'(?i)(offline|replace|rebuild|erase|firmware|initialize|delete)[[:space:]]*(=|:|/)' cmd internal packaging scripts; then
	echo "potential dangerous controller command constructor found" >&2
	exit 1
fi

for package in dist/packages/zfs-pay_*.deb; do
	test -f "$package"
	if command -v dpkg-deb >/dev/null 2>&1; then
		./scripts/package/verify-deb.sh "$package"
	else
		docker run --rm -v "$ROOT:/src:ro" -w /src debian:bookworm-slim sh -c \
			'apt-get update -qq && apt-get install -y -qq file >/dev/null && ./scripts/package/verify-deb.sh "$1"' \
			sh "$package"
	fi
done

mkdir -p dist/release
cp dist/packages/zfs-pay_*.deb dist/packages/SHA256SUMS dist/release/
cat >dist/release/THIRD_PARTY.md <<'EOF'
# Third-party components

The zfs-pay Go binaries use only the Go standard library. Broadcom StorCLI,
Dell PERCCLI, OpenZFS tools, ledctl, Linux, systemd, and Debian are external
runtime or platform components and are not included in these artifacts.
EOF

cat >dist/release/SBOM.spdx.json <<'EOF'
{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "zfs-pay-candidate",
  "documentNamespace": "https://github.com/zoujunkun/zfs-pay/sbom/candidate-local",
  "creationInfo": {
    "created": "1970-01-01T00:00:00Z",
    "creators": ["Tool: zfs-pay-release-audit"]
  },
  "packages": [
    {
      "name": "zfs-pay",
      "SPDXID": "SPDXRef-Package-zfs-pay",
      "downloadLocation": "NOASSERTION",
      "filesAnalyzed": false,
      "licenseConcluded": "MIT",
      "licenseDeclared": "MIT",
      "copyrightText": "NOASSERTION",
      "externalRefs": []
    }
  ],
  "relationships": [
    {
      "spdxElementId": "SPDXRef-DOCUMENT",
      "relationshipType": "DESCRIBES",
      "relatedSpdxElement": "SPDXRef-Package-zfs-pay"
    }
  ]
}
EOF

python3 -m json.tool dist/release/SBOM.spdx.json >/dev/null
