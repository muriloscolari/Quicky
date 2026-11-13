.pragma library

function request(baseUrl, method, path, payload, callback) {
    var xhr = new XMLHttpRequest();
    var cleanBase = (baseUrl || "http://127.0.0.1:47892").replace(/\/+$/, "");
    xhr.open(method, cleanBase + path);
    xhr.setRequestHeader("Content-Type", "application/json");
    xhr.onreadystatechange = function() {
        if (xhr.readyState !== XMLHttpRequest.DONE) {
            return;
        }
        if (xhr.status === 0) {
            callback("Daemon QuiCkY offline em " + cleanBase, null);
            return;
        }
        var parsed = {};
        try {
            parsed = xhr.responseText ? JSON.parse(xhr.responseText) : {};
        } catch (e) {
            callback("Resposta inválida do daemon", null);
            return;
        }
        if (xhr.status >= 200 && xhr.status < 300) {
            callback(null, parsed);
        } else {
            callback(parsed.error || ("Erro HTTP " + xhr.status), parsed);
        }
    };
    xhr.send(payload ? JSON.stringify(payload) : null);
}

function getStatus(baseUrl, callback) {
    request(baseUrl, "GET", "/api/status", null, callback);
}

function startScan(baseUrl, callback) {
    request(baseUrl, "POST", "/api/scan", {}, callback);
}

function connectDevice(baseUrl, addresses, callback) {
    request(baseUrl, "POST", "/api/connect", { addresses: addresses }, callback);
}

function disconnectDevice(baseUrl, callback) {
    request(baseUrl, "POST", "/api/disconnect", {}, callback);
}

function refreshDevice(baseUrl, callback) {
    request(baseUrl, "POST", "/api/refresh", {}, callback);
}

function sendANC(baseUrl, cmdId, modeKey, callback) {
    var payload = (typeof cmdId === "number" && !isNaN(cmdId))
        ? { action: "anc", cmdId: cmdId }
        : { action: "anc", mode: modeKey };
    request(baseUrl, "POST", "/api/control", payload, callback);
}

function setAutoConnect(baseUrl, enabled, callback) {
    request(baseUrl, "POST", "/api/autoconnect", { enabled: enabled }, callback);
}
