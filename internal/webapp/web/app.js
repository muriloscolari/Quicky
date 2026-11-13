const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => [...document.querySelectorAll(selector)];

const elements = {
  statusDot: $("#statusDot"),
  systemStatus: $("#systemStatus"),
  scanButton: $("#scanButton"),
  scanHint: $("#scanHint"),
  deviceList: $("#deviceList"),
  manualForm: $("#manualForm"),
  manualAddress: $("#manualAddress"),
  dashboard: $("#dashboard"),
  deviceTitle: $("#deviceTitle"),
  deviceMeta: $("#deviceMeta"),
  connectedAddress: $("#connectedAddress"),
  deviceName: $("#deviceName"),
  firmwareLeft: $("#firmwareLeft"),
  firmwareRight: $("#firmwareRight"),
  refreshButton: $("#refreshButton"),
  disconnectButton: $("#disconnectButton"),
  featureNotice: $("#featureNotice"),
  ancSupport: $("#ancSupport"),
  ancControls: $("#ancControls"),
  ancSubControls: $("#ancSubControls"),
  ancLevelControl: $("#ancLevelControl"),
  toast: $("#toast"),
};

let currentStatus = null;
let selectedANCCmdID = null;
let toastTimer = null;

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || `Erro HTTP ${response.status}`);
  return data;
}

function toast(message, error = false) {
  clearTimeout(toastTimer);
  elements.toast.textContent = message;
  elements.toast.className = `toast visible${error ? " error" : ""}`;
  toastTimer = setTimeout(() => { elements.toast.className = "toast"; }, 5000);
}

function escapeHTML(value = "") {
  return String(value).replace(/[&<>'"]/g, (char) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;",
  })[char]);
}

function productName(device) {
  return device?.product?.title || device?.name || "QCY não identificado";
}

function renderStatus(status) {
  currentStatus = status;
  const busy = status.scanning || status.connecting;
  elements.statusDot.className = `status-dot${status.connection.connected ? " active" : busy ? " busy" : ""}`;
  elements.systemStatus.textContent = status.connection.connected
    ? `Conectado · ${status.connection.address}`
    : status.connecting ? "Conectando via GATT" : status.scanning ? "Procurando anúncios QCY" : "Bluetooth aguardando";

  elements.scanButton.disabled = busy || status.connection.connected;
  elements.scanButton.classList.toggle("scanning", status.scanning);
  elements.scanButton.querySelector("span:last-child").textContent = status.scanning ? "Procurando..." : "Procurar QCY";
  elements.deviceList.classList.toggle("scanning", status.scanning);
  renderDevices(status.devices, status.scanning, status.connecting);
  renderConnection(status.connection);
}

function renderDevices(devices, scanning, connecting) {
  if (!devices.length) {
    elements.deviceList.innerHTML = `
      <div class="empty-state">
        <div class="radar" aria-hidden="true"><i></i></div>
        <strong>${scanning ? "Procurando sinais QCY..." : "Nenhum QCY encontrado"}</strong>
        <span>${scanning ? "A busca dura aproximadamente 12 segundos." : "Abra o estojo, retire os fones e tente novamente."}</span>
      </div>`;
    return;
  }

  elements.deviceList.innerHTML = devices.map((device, index) => {
    const strength = device.rssi >= -60 ? "" : device.rssi >= -75 ? "medium" : "weak";
    const subtitle = device.product?.subTitle || `Vendor ID ${device.vendorId}`;
    // The advertised control address is commonly the Classic/A2DP address.
    // Connecting to it as BLE can interrupt audio, so discovery uses only the
    // actual address that emitted the BLE advertisement.
    const addresses = [device.bleAddress].filter(Boolean);
    return `
      <article class="device-item">
        <div>
          <h3>${escapeHTML(productName(device))}</h3>
          <p class="subtitle">${escapeHTML(subtitle)}</p>
          <span class="mac">CTRL ${escapeHTML(device.controlAddress || "indisponível")}</span><br>
          <span class="mac">BLE&nbsp; ${escapeHTML(device.bleAddress)}</span>
          <div class="device-battery">
            <span>L <b>${device.leftBattery}%</b></span>
            <span>R <b>${device.rightBattery}%</b></span>
            <span>CASE <b>${device.caseBattery}%</b></span>
          </div>
          <button class="button button-secondary connect-device" type="button" data-index="${index}" ${connecting ? "disabled" : ""}>Conectar</button>
          <script type="application/json" class="device-addresses">${JSON.stringify(addresses)}</script>
        </div>
        <div title="RSSI ${device.rssi} dBm">
          <div class="signal ${strength}"><i></i><i></i><i></i><i></i></div>
          <span class="mac">${device.rssi}</span>
        </div>
      </article>`;
  }).join("");

  $$(".connect-device").forEach((button) => {
    button.addEventListener("click", () => {
      const card = button.closest(".device-item");
      const addresses = JSON.parse(card.querySelector(".device-addresses").textContent);
      connect(addresses);
    });
  });
}

function renderConnection(connection) {
  elements.dashboard.hidden = !connection.connected;
  if (!connection.connected) return;

  const device = connection.device;
  elements.deviceTitle.textContent = productName(device);
  elements.deviceMeta.textContent = `${connection.address} · conectado ${new Date(connection.since).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })}`;
  elements.connectedAddress.textContent = connection.address || "--";
  if (document.activeElement !== elements.deviceName) {
    elements.deviceName.value = device?.product?.title || device?.name || "";
  }
  elements.firmwareLeft.textContent = connection.firmware?.left || "--";
  elements.firmwareRight.textContent = connection.firmware?.right || "--";
  const battery = connection.battery || (device ? {
    left: device.leftBattery,
    right: device.rightBattery,
    case: device.caseBattery,
    leftCharging: device.leftCharging,
    rightCharging: device.rightCharging,
    caseCharging: device.caseCharging,
  } : null);
  renderBattery(battery);

  const hasKnownProduct = Boolean(device?.product);
  const hasANC = Boolean(device?.product?.features?.anc);
  elements.ancSupport.textContent = hasANC ? "marcado como suportado" : "não confirmado neste modelo";
  elements.featureNotice.textContent = hasKnownProduct
    ? `Modelo identificado na base QCY. Envie somente recursos que seu firmware suporta; um comando aceito pelo Linux ainda pode ser ignorado pelo fone.`
    : `Modelo não identificado na base. Os controles permanecem disponíveis para diagnóstico, mas podem não produzir efeito.`;
  renderANCControls(device, connection.anc);
}

const ancLabelMap = {
  "Noise canceling": "ANC",
  "Transparency": "Transparência",
  "Normal": "Normal",
  "Off": "Normal",
  "Outdoor": "Exterior",
  "Adaptive Noise Cancellation": "Adaptativo",
  "Indoor": "Interno",
  "Commuting": "Transporte",
  "Noisy": "Ruidoso",
  "Anti-wind noise": "Anti-vento",
};

function ancLabel(name) {
  return ancLabelMap[name] || name;
}

function cmdInRange(cmdId, start, end) {
  return Number.isFinite(cmdId) && cmdId >= start && cmdId <= (end || start);
}

async function sendANC(cmdId, modeKey) {
  const payload = Number.isFinite(cmdId)
    ? { action: "anc", cmdId }
    : { action: "anc", mode: modeKey };
  const ok = await control(payload, "Modo de ambiente enviado.");
  if (ok) {
    if (Number.isFinite(cmdId)) selectedANCCmdID = cmdId;
    await updateStatus();
  }
  return ok;
}

function renderANCControls(device, ancState) {
  const modes = device?.product?.features?.anc?.modes;
  if (Number.isFinite(ancState?.cmdId)) {
    selectedANCCmdID = ancState.cmdId;
  }

  if (!modes || modes.length === 0) {
    elements.ancSubControls.hidden = true;
    elements.ancSubControls.innerHTML = "";
    elements.ancLevelControl.hidden = true;
    elements.ancLevelControl.innerHTML = "";
    const fallbackModes = [
      { mode: "off", label: "Normal" },
      { mode: "anc", label: "ANC" },
      { mode: "transparency", label: "Transparência" },
      { mode: "outdoor", label: "Exterior" },
    ];
    elements.ancControls.innerHTML = fallbackModes.map((item) => `
      <button type="button" data-mode="${item.mode}">${item.label}</button>
    `).join("");
    $$("#ancControls button").forEach((button) => {
      button.addEventListener("click", async () => {
        const ok = await sendANC(null, button.dataset.mode);
        if (ok) {
          $$("#ancControls button").forEach((item) => item.classList.remove("active"));
          button.classList.add("active");
        }
      });
    });
    return;
  }

  let activeMode = modes.find((m) =>
    cmdInRange(selectedANCCmdID, m.startcmdid, m.endcmdid) ||
    (m.items || []).some((it) => cmdInRange(selectedANCCmdID, it.startcmdid, it.endcmdid))
  );

  elements.ancControls.innerHTML = modes.map((mode, index) => {
    const cmdId = mode.defaultcmd || mode.startcmdid;
    const isActive = activeMode === mode;
    return `<button type="button" data-cmd-id="${cmdId}" data-mode-index="${index}" class="${isActive ? "active" : ""}">${escapeHTML(ancLabel(mode.name))}</button>`;
  }).join("");

  $$("#ancControls button").forEach((button) => {
    button.addEventListener("click", () => {
      sendANC(Number(button.dataset.cmdId));
    });
  });

  const items = activeMode?.items || [];
  let activeItem = null;
  if (items.length > 0) {
    activeItem = items.find((it) => cmdInRange(selectedANCCmdID, it.startcmdid, it.endcmdid)) || items[0];
    elements.ancSubControls.hidden = false;
    elements.ancSubControls.innerHTML = items.map((item) => {
      const cmdId = item.startcmdid;
      const isActive = activeItem === item;
      return `<button type="button" data-cmd-id="${cmdId}" class="${isActive ? "active" : ""}">${escapeHTML(ancLabel(item.name))}</button>`;
    }).join("");
    $$("#ancSubControls button").forEach((button) => {
      button.addEventListener("click", () => {
        sendANC(Number(button.dataset.cmdId));
      });
    });
  } else {
    elements.ancSubControls.hidden = true;
    elements.ancSubControls.innerHTML = "";
  }

  const rangeSource = activeItem || activeMode;
  if (rangeSource && rangeSource.endcmdid > rangeSource.startcmdid && (rangeSource.endcmdid - rangeSource.startcmdid) <= 10) {
    const count = rangeSource.endcmdid - rangeSource.startcmdid + 1;
    const levels = Array.from({ length: count }, (_, idx) => {
      const cmdId = rangeSource.startcmdid + idx;
      const isActive = selectedANCCmdID === cmdId;
      return `<button type="button" data-cmd-id="${cmdId}" class="${isActive ? "active" : ""}">${idx + 1}</button>`;
    }).join("");
    elements.ancLevelControl.hidden = false;
    elements.ancLevelControl.innerHTML = `<span>Intensidade</span>${levels}`;
    $$("#ancLevelControl button").forEach((button) => {
      button.addEventListener("click", () => {
        sendANC(Number(button.dataset.cmdId));
      });
    });
  } else {
    elements.ancLevelControl.hidden = true;
    elements.ancLevelControl.innerHTML = "";
  }
}

function renderBattery(battery) {
  const values = [
    ["Left", battery?.left, battery?.leftCharging],
    ["Right", battery?.right, battery?.rightCharging],
    ["Case", battery?.case, battery?.caseCharging],
  ];
  values.forEach(([side, level, charging]) => {
    const display = $(`#battery${side}`);
    const bar = $(`#battery${side}Bar`);
    const state = $(`#battery${side}State`);
    display.textContent = Number.isFinite(level) ? level : "--";
    bar.style.width = Number.isFinite(level) ? `${Math.min(100, level)}%` : "0";
    state.textContent = Number.isFinite(level) ? (charging ? "carregando" : "em uso") : "sem leitura";
  });
}

async function updateStatus(showError = false) {
  try {
    renderStatus(await api("/api/status"));
  } catch (error) {
    elements.statusDot.className = "status-dot";
    elements.systemStatus.textContent = "Backend indisponível";
    if (showError) toast(error.message, true);
  }
}

async function scan() {
  try {
    await api("/api/scan", { method: "POST", body: "{}" });
    await updateStatus();
  } catch (error) {
    toast(error.message, true);
  }
}

async function connect(addresses) {
  elements.scanButton.disabled = true;
  toast("Conectando ao serviço GATT do fone...");
  try {
    const connection = await api("/api/connect", { method: "POST", body: JSON.stringify({ addresses }) });
    await updateStatus();
    elements.dashboard.scrollIntoView({ behavior: "smooth", block: "start" });
    if (connection.warning) {
      toast(connection.warning, true);
    } else if (connection.audioReconnected) {
      toast("Controle BLE conectado e áudio restaurado.");
    } else {
      toast("Fone conectado. Controles BLE disponíveis.");
    }
  } catch (error) {
    toast(error.message, true);
    await updateStatus();
  }
}

async function control(payload, successMessage) {
  try {
    await api("/api/control", { method: "POST", body: JSON.stringify(payload) });
    if (successMessage) toast(successMessage);
    return true;
  } catch (error) {
    toast(error.message, true);
    return false;
  }
}

elements.scanButton.addEventListener("click", scan);

elements.manualForm.addEventListener("submit", (event) => {
  event.preventDefault();
  const address = elements.manualAddress.value.trim();
  if (!/^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(address)) {
    toast("Use um MAC no formato AA:BB:CC:DD:EE:FF", true);
    return;
  }
  connect([address]);
});

elements.disconnectButton.addEventListener("click", async () => {
  try {
    await api("/api/disconnect", { method: "POST", body: "{}" });
    selectedANCCmdID = null;
    $$('[data-toggle]').forEach((input) => { input.checked = false; });
    $$("#ancControls button").forEach((button) => button.classList.remove("active"));
    await updateStatus();
    toast("Fone desconectado do canal de controle.");
  } catch (error) {
    toast(error.message, true);
  }
});

elements.refreshButton.addEventListener("click", async () => {
  elements.refreshButton.disabled = true;
  try {
    await api("/api/refresh", { method: "POST", body: "{}" });
    await updateStatus();
    toast("Bateria e firmware atualizados.");
  } catch (error) {
    toast(error.message, true);
  } finally {
    elements.refreshButton.disabled = false;
  }
});

$$('[data-toggle]').forEach((input) => {
  input.addEventListener("change", async () => {
    const ok = await control({ action: input.dataset.toggle, enabled: input.checked }, "Configuração enviada.");
    if (!ok) input.checked = !input.checked;
  });
});

function bindRange(id, formatter = (value) => value) {
  const input = $(`#${id}`);
  const output = $(`#${id}Output`);
  input.addEventListener("input", () => { output.value = formatter(Number(input.value)); });
}

bindRange("leftVolume");
bindRange("rightVolume");
bindRange("toneVolume");
bindRange("balance", (value) => value === 50 ? "Centro" : value < 50 ? `L ${50 - value}` : `R ${value - 50}`);

$("#applyVolume").addEventListener("click", () => control({
  action: "volume",
  left: Number($("#leftVolume").value),
  right: Number($("#rightVolume").value),
}, "Volume enviado aos dois canais."));

$("#balance").addEventListener("change", (event) => control({
  action: "balance", value: Number(event.target.value),
}, "Balanço enviado."));

$("#toneVolume").addEventListener("change", (event) => control({
  action: "tone_volume", value: Number(event.target.value),
}, "Volume dos avisos enviado."));

$("#renameForm").addEventListener("submit", async (event) => {
  event.preventDefault();
  const name = elements.deviceName.value.trim();
  if (!name) return toast("Informe um nome para o dispositivo.", true);
  await control({ action: "rename", name }, "Novo nome enviado ao fone.");
});

updateStatus(true);
setInterval(() => updateStatus(false), 1500);
