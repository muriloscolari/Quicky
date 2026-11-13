import QtQuick
import QtQuick.Layouts
import QtQuick.Controls as QQC2
import org.kde.plasma.plasmoid
import org.kde.kirigami as Kirigami
import "ApiClient.js" as ApiClient

PlasmoidItem {
    id: root

    property string daemonUrl: Plasmoid.configuration.daemonUrl || "http://127.0.0.1:47892"
    property int pollIntervalMs: Plasmoid.configuration.pollIntervalMs || 5000

    property bool daemonOnline: false
    property bool scanning: false
    property bool connecting: false
    property bool busyAction: false
    property string errorMessage: ""
    property var devices: []
    property var connection: ({ connected: false })
    property int localAncCmdId: -1

    readonly property bool isConnected: Boolean(connection && connection.connected)
    readonly property var activeDevice: connection ? connection.device : null
    readonly property var activeBattery: (connection && connection.battery)
        ? connection.battery
        : (activeDevice ? {
            left: activeDevice.leftBattery,
            right: activeDevice.rightBattery,
            case: activeDevice.caseBattery,
            leftCharging: activeDevice.leftCharging,
            rightCharging: activeDevice.rightCharging,
            caseCharging: activeDevice.caseCharging
        } : null)
    readonly property int minEarbudBattery: {
        if (!isConnected || !activeBattery) return -1;
        var l = typeof activeBattery.left === "number" ? activeBattery.left : -1;
        var r = typeof activeBattery.right === "number" ? activeBattery.right : -1;
        if (l >= 0 && r >= 0) return Math.min(l, r);
        return l >= 0 ? l : r;
    }

    toolTipMainText: isConnected
        ? ((activeDevice && activeDevice.product && activeDevice.product.title) || "QCY Conectado")
        : "QuiCkY QCY"
    toolTipSubText: {
        if (!daemonOnline) return "Daemon offline (" + daemonUrl + ")";
        if (!isConnected) return "Desconectado · clique para procurar";
        if (!activeBattery) return "Conectado · sem leitura de bateria";
        return "L: " + activeBattery.left + "%  R: " + activeBattery.right + "%  Estojo: " + activeBattery.case + "%";
    }

    function applyStatus(status) {
        daemonOnline = true;
        scanning = Boolean(status.scanning);
        connecting = Boolean(status.connecting);
        devices = status.devices || [];
        connection = status.connection || { connected: false };
        if (connection && connection.anc && typeof connection.anc.cmdId === "number") {
            localAncCmdId = connection.anc.cmdId;
        } else if (!isConnected) {
            localAncCmdId = -1;
        }
        if (status.lastError) {
            errorMessage = status.lastError;
        }
    }

    function pollStatus() {
        ApiClient.getStatus(daemonUrl, function(err, data) {
            if (err) {
                daemonOnline = false;
                errorMessage = err;
                return;
            }
            errorMessage = "";
            applyStatus(data);
        });
    }

    Component.onCompleted: pollStatus()

    Timer {
        interval: root.scanning ? 1500 : root.pollIntervalMs
        running: true
        repeat: true
        onTriggered: root.pollStatus()
    }

    compactRepresentation: MouseArea {
        id: compactRoot
        implicitWidth: compactRow.implicitWidth + 8
        implicitHeight: compactRow.implicitHeight + 4
        onClicked: root.expanded = !root.expanded

        RowLayout {
            id: compactRow
            anchors.centerIn: parent
            spacing: 4

            Kirigami.Icon {
                source: "audio-headphones"
                implicitWidth: Kirigami.Units.iconSizes.smallMedium
                implicitHeight: Kirigami.Units.iconSizes.smallMedium
                color: root.isConnected ? "#3daee9" : (root.daemonOnline ? Kirigami.Theme.textColor : "#7f8c8d")
            }

            QQC2.Label {
                visible: root.isConnected && root.minEarbudBattery >= 0
                text: root.minEarbudBattery + "%"
                font.pixelSize: 11
                font.weight: Font.Bold
                color: root.minEarbudBattery <= 20 ? "#da4453" : "#eff0f1"
            }
        }
    }

    fullRepresentation: Rectangle {
        id: popupRoot
        implicitWidth: 360
        implicitHeight: Math.min(540, Math.max(260, contentCol.implicitHeight + 28))
        Layout.minimumWidth: 340
        Layout.minimumHeight: 240
        color: "#1b1e20"
        radius: 12
        border.width: 1
        border.color: "#31363b"

        ColumnLayout {
            id: contentCol
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: 14
            spacing: 12

            // Cabeçalho
            RowLayout {
                Layout.fillWidth: true
                spacing: 8

                Rectangle {
                    width: 34
                    height: 34
                    radius: 9
                    color: root.isConnected ? "#223b49" : "#232629"
                    border.width: 1
                    border.color: root.isConnected ? "#3daee9" : "#31363b"

                    Kirigami.Icon {
                        anchors.centerIn: parent
                        source: "audio-headphones"
                        implicitWidth: 18
                        implicitHeight: 18
                        color: root.isConnected ? "#3daee9" : "#9ea7b0"
                    }
                }

                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 1
                    QQC2.Label {
                        text: root.isConnected
                            ? ((root.activeDevice && root.activeDevice.product && root.activeDevice.product.title) || "QCY Conectado")
                            : "QuiCkY QCY"
                        color: "#eff0f1"
                        font.pixelSize: 14
                        font.weight: Font.Bold
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                    QQC2.Label {
                        text: !root.daemonOnline
                            ? "Daemon offline"
                            : (root.isConnected ? (root.connection.address || "BLE conectado") : (root.scanning ? "Procurando QCY..." : "Pronto para conectar"))
                        color: root.isConnected ? "#3daee9" : "#9ea7b0"
                        font.pixelSize: 11
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                }

                QQC2.ToolButton {
                    visible: root.isConnected
                    enabled: !root.busyAction
                    icon.name: "view-refresh"
                    QQC2.ToolTip.visible: hovered
                    QQC2.ToolTip.text: "Atualizar bateria"
                    onClicked: {
                        root.busyAction = true;
                        ApiClient.refreshDevice(root.daemonUrl, function(err) {
                            root.busyAction = false;
                            if (err) root.errorMessage = err;
                            root.pollStatus();
                        });
                    }
                }

                QQC2.ToolButton {
                    visible: root.isConnected
                    enabled: !root.busyAction
                    icon.name: "network-disconnect"
                    QQC2.ToolTip.visible: hovered
                    QQC2.ToolTip.text: "Desconectar"
                    onClicked: {
                        root.busyAction = true;
                        ApiClient.disconnectDevice(root.daemonUrl, function(err) {
                            root.busyAction = false;
                            if (err) root.errorMessage = err;
                            root.pollStatus();
                        });
                    }
                }
            }

            // Aviso de erro / daemon offline
            Rectangle {
                visible: !root.daemonOnline || root.errorMessage !== ""
                Layout.fillWidth: true
                radius: 8
                color: "#331d21"
                border.width: 1
                border.color: "#da4453"
                implicitHeight: errLbl.implicitHeight + 14

                QQC2.Label {
                    id: errLbl
                    anchors.fill: parent
                    anchors.margins: 7
                    wrapMode: Text.WordWrap
                    color: "#f5b7be"
                    font.pixelSize: 11
                    text: !root.daemonOnline
                        ? "Daemon indisponível em " + root.daemonUrl + ". Verifique: systemctl --user status quicky-daemon"
                        : root.errorMessage
                }
            }

            // Painel quando CONECTADO: Bateria + ANC
            ColumnLayout {
                visible: root.isConnected
                Layout.fillWidth: true
                spacing: 10

                RowLayout {
                    Layout.fillWidth: true
                    spacing: 8

                    BatteryCard {
                        Layout.fillWidth: true
                        label: "ESQUERDO"
                        level: root.activeBattery ? root.activeBattery.left : -1
                        charging: root.activeBattery ? Boolean(root.activeBattery.leftCharging) : false
                    }
                    BatteryCard {
                        Layout.fillWidth: true
                        label: "DIREITO"
                        level: root.activeBattery ? root.activeBattery.right : -1
                        charging: root.activeBattery ? Boolean(root.activeBattery.rightCharging) : false
                    }
                    BatteryCard {
                        Layout.fillWidth: true
                        label: "ESTOJO"
                        level: root.activeBattery ? root.activeBattery.case : -1
                        charging: root.activeBattery ? Boolean(root.activeBattery.caseCharging) : false
                    }
                }

                AncSelector {
                    Layout.fillWidth: true
                    productModes: (root.activeDevice && root.activeDevice.product && root.activeDevice.product.features && root.activeDevice.product.features.anc)
                        ? root.activeDevice.product.features.anc.modes
                        : []
                    currentCmdId: root.localAncCmdId
                    busy: root.busyAction
                    onSelectANC: function(cmdId, modeKey) {
                        root.busyAction = true;
                        root.errorMessage = "";
                        ApiClient.sendANC(root.daemonUrl, cmdId, modeKey, function(err) {
                            root.busyAction = false;
                            if (err) {
                                root.errorMessage = err;
                            } else if (typeof cmdId === "number" && !isNaN(cmdId)) {
                                root.localAncCmdId = cmdId;
                            }
                            root.pollStatus();
                        });
                    }
                }
            }

            // Painel quando DESCONECTADO: Scan e Lista
            ColumnLayout {
                visible: !root.isConnected && root.daemonOnline
                Layout.fillWidth: true
                spacing: 8

                QQC2.Button {
                    Layout.fillWidth: true
                    implicitHeight: 38
                    enabled: !root.scanning && !root.connecting
                    text: root.connecting ? "Conectando..." : (root.scanning ? "Procurando QCY..." : "Procurar QCY")

                    background: Rectangle {
                        radius: 9
                        color: parent.enabled ? (parent.hovered ? "#4ab8f0" : "#3daee9") : "#2a2e32"
                    }
                    contentItem: QQC2.Label {
                        text: parent.text
                        horizontalAlignment: Text.AlignHCenter
                        verticalAlignment: Text.AlignVCenter
                        color: parent.enabled ? "#1b1e20" : "#9ea7b0"
                        font.pixelSize: 12
                        font.weight: Font.Bold
                    }
                    onClicked: {
                        root.errorMessage = "";
                        ApiClient.startScan(root.daemonUrl, function(err) {
                            if (err) root.errorMessage = err;
                            root.pollStatus();
                        });
                    }
                }

                Repeater {
                    model: root.devices
                    delegate: Rectangle {
                        required property var modelData
                        Layout.fillWidth: true
                        radius: 9
                        color: "#232629"
                        border.width: 1
                        border.color: "#31363b"
                        implicitHeight: devRow.implicitHeight + 16

                        RowLayout {
                            id: devRow
                            anchors.fill: parent
                            anchors.margins: 8
                            spacing: 8

                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: 2
                                QQC2.Label {
                                    text: (modelData.product && modelData.product.title) || modelData.name || "QCY"
                                    color: "#eff0f1"
                                    font.pixelSize: 12
                                    font.weight: Font.Bold
                                    elide: Text.ElideRight
                                    Layout.fillWidth: true
                                }
                                QQC2.Label {
                                    text: "L " + modelData.leftBattery + "% · R " + modelData.rightBattery + "% · " + modelData.bleAddress
                                    color: "#9ea7b0"
                                    font.pixelSize: 10
                                    elide: Text.ElideRight
                                    Layout.fillWidth: true
                                }
                            }

                            QQC2.Button {
                                text: "Conectar"
                                enabled: !root.connecting
                                implicitHeight: 30
                                background: Rectangle {
                                    radius: 7
                                    color: parent.hovered ? "#3daee9" : "#2e3338"
                                    border.width: 1
                                    border.color: "#3daee9"
                                }
                                contentItem: QQC2.Label {
                                    text: parent.text
                                    color: parent.hovered ? "#1b1e20" : "#eff0f1"
                                    font.pixelSize: 11
                                    font.weight: Font.DemiBold
                                    horizontalAlignment: Text.AlignHCenter
                                    verticalAlignment: Text.AlignVCenter
                                }
                                onClicked: {
                                    root.errorMessage = "";
                                    ApiClient.connectDevice(root.daemonUrl, [modelData.bleAddress], function(err) {
                                        if (err) root.errorMessage = err;
                                        root.pollStatus();
                                    });
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
