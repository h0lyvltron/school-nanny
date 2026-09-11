#!/usr/bin/env bash
# Track 2 LAN TLS: mkcert CA + leaf for school-nanny.home
#
# Run (no root required for cert files):
#   ./scripts/setup-lan-certs.sh
#
# Optional (trust on THIS Linux machine's browsers):
#   sudo env PATH="$HOME/.local/bin:$PATH" CAROOT="$HOME/.local/share/mkcert" mkcert -install
set -euo pipefail

HOSTNAME_LAN="${HOSTNAME_LAN:-school-nanny.home}"
BIN_DIR="${HOME}/.local/bin"
export CAROOT="${CAROOT:-${HOME}/.local/share/mkcert}"
CERT_DIR="${CERT_DIR:-${HOME}/.local/share/school-nanny/certs}"
MKCERT_VER="${MKCERT_VER:-v1.4.4}"

mkdir -p "${BIN_DIR}" "${CAROOT}" "${CERT_DIR}"
export PATH="${BIN_DIR}:${PATH}"

if ! command -v mkcert >/dev/null 2>&1; then
  ARCH=$(uname -m)
  case "${ARCH}" in
    x86_64) MKCERT_ARCH=amd64 ;;
    aarch64|arm64) MKCERT_ARCH=arm64 ;;
    *) echo "unsupported arch: ${ARCH}" >&2; exit 1 ;;
  esac
  URL="https://github.com/FiloSottile/mkcert/releases/download/${MKCERT_VER}/mkcert-${MKCERT_VER}-linux-${MKCERT_ARCH}"
  echo "==> Installing mkcert ${MKCERT_VER} to ${BIN_DIR}/mkcert"
  curl -fsSL "${URL}" -o "${BIN_DIR}/mkcert"
  chmod +x "${BIN_DIR}/mkcert"
fi

echo "==> mkcert $(mkcert -version)  CAROOT=${CAROOT}"

if [[ ! -f "${CAROOT}/rootCA.pem" ]]; then
  echo "==> Creating local CA (not yet in system trust store)..."
  # Generate a throwaway leaf so mkcert creates the CA without requiring sudo.
  tmp=$(mktemp -d)
  (cd "${tmp}" && mkcert -cert-file t.pem -key-file t-key.pem localhost >/dev/null)
  rm -rf "${tmp}"
fi

echo "==> Issuing leaf for ${HOSTNAME_LAN}..."
(
  cd "${CERT_DIR}"
  mkcert -cert-file "${HOSTNAME_LAN}.pem" -key-file "${HOSTNAME_LAN}-key.pem" \
    "${HOSTNAME_LAN}" "*.${HOSTNAME_LAN}" localhost 127.0.0.1 ::1
)
cp -f "${CAROOT}/rootCA.pem" "${CERT_DIR}/rootCA.pem"
chmod 644 "${CERT_DIR}/${HOSTNAME_LAN}.pem" "${CERT_DIR}/rootCA.pem"
chmod 600 "${CERT_DIR}/${HOSTNAME_LAN}-key.pem"

echo
echo "Files:"
ls -la "${CERT_DIR}"
echo
echo "Done (C1–C3)."
echo
echo "Next — trust the CA on her devices (C4/C5):"
echo "  Windows: copy ${CERT_DIR}/rootCA.pem then run scripts/trust-mkcert-windows.ps1"
echo "  iPad:    AirDrop rootCA.pem → install profile → Certificate Trust Settings → enable full trust"
echo
echo "Optional — trust on this Linux box:"
echo "  sudo env PATH=\"${BIN_DIR}:\$PATH\" CAROOT=\"${CAROOT}\" mkcert -install"
echo
echo "Coolify / proxy (C6): custom cert = ${CERT_DIR}/${HOSTNAME_LAN}.pem"
echo "                      key         = ${CERT_DIR}/${HOSTNAME_LAN}-key.pem"
echo "Do not commit or share rootCA-key.pem or *-key.pem."
