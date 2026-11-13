#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${HOME}/.local/bin"
SYSTEMD_DIR="${HOME}/.config/systemd/user"
PLASMOID_DIR="${ROOT_DIR}/plasmoid"
PLASMOID_ID="me.muriloscolari.quicky"

echo "[1/3] Compilando e instalando quicky-control em ${BIN_DIR}..."
mkdir -p "${BIN_DIR}" "${ROOT_DIR}/bin"
(cd "${ROOT_DIR}" && go build -o bin/quicky-control ./cli)
install -m 0755 "${ROOT_DIR}/bin/quicky-control" "${BIN_DIR}/quicky-control"

echo "[2/3] Instalando e iniciando quicky-daemon.service (systemd --user)..."
mkdir -p "${SYSTEMD_DIR}"
install -m 0644 "${ROOT_DIR}/systemd/quicky-daemon.service" "${SYSTEMD_DIR}/quicky-daemon.service"
systemctl --user daemon-reload
systemctl --user enable quicky-daemon.service
systemctl --user restart quicky-daemon.service

echo "[3/3] Instalando/atualizando Plasmoid KDE (${PLASMOID_ID})..."
if kpackagetool6 --type Plasma/Applet --show "${PLASMOID_ID}" >/dev/null 2>&1; then
  kpackagetool6 --type Plasma/Applet --upgrade "${PLASMOID_DIR}"
else
  kpackagetool6 --type Plasma/Applet --install "${PLASMOID_DIR}"
fi

echo "Pronto! Daemon ativo em http://127.0.0.1:47892 e Plasmoid instalado."
