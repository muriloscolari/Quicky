import QtQuick
import QtQuick.Layouts
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami

Rectangle {
    id: root
    property var productModes: []
    property int currentCmdId: -1
    property bool busy: false
    signal selectANC(var cmdId, string modeKey)

    radius: 10
    color: "#232629"
    border.width: 1
    border.color: "#31363b"
    implicitHeight: mainCol.implicitHeight + 24

    readonly property var labelMap: ({
        "Noise canceling": "ANC",
        "Transparency": "Transparência",
        "Normal": "Normal",
        "Off": "Normal",
        "Outdoor": "Exterior",
        "Adaptive Noise Cancellation": "Adaptativo",
        "Indoor": "Interno",
        "Commuting": "Transporte",
        "Noisy": "Ruidoso",
        "Anti-wind noise": "Anti-vento"
    })

    function translateLabel(name) {
        return labelMap[name] || name;
    }

    function inRange(cmdId, start, end) {
        return cmdId >= 0 && cmdId >= start && cmdId <= (end || start);
    }

    function findActiveMode() {
        if (!productModes || productModes.length === 0) return null;
        for (var i = 0; i < productModes.length; i++) {
            var m = productModes[i];
            if (inRange(currentCmdId, m.startcmdid, m.endcmdid)) return m;
            if (m.items) {
                for (var j = 0; j < m.items.length; j++) {
                    if (inRange(currentCmdId, m.items[j].startcmdid, m.items[j].endcmdid)) {
                        return m;
                    }
                }
            }
        }
        return null;
    }

    function findActiveItem(mode) {
        if (!mode || !mode.items || mode.items.length === 0) return null;
        for (var j = 0; j < mode.items.length; j++) {
            if (inRange(currentCmdId, mode.items[j].startcmdid, mode.items[j].endcmdid)) {
                return mode.items[j];
            }
        }
        return mode.items[0];
    }

    property var activeMode: findActiveMode()
    property var activeItem: findActiveItem(activeMode)
    property var rangeSource: activeItem || activeMode
    property int levelCount: (rangeSource && rangeSource.endcmdid > rangeSource.startcmdid && (rangeSource.endcmdid - rangeSource.startcmdid) <= 10)
        ? (rangeSource.endcmdid - rangeSource.startcmdid + 1)
        : 0

    ColumnLayout {
        id: mainCol
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 12
        spacing: 10

        RowLayout {
            Layout.fillWidth: true
            QQC2.Label {
                text: "AMBIENTE (ANC)"
                color: "#9ea7b0"
                font.pixelSize: 10
                font.weight: Font.DemiBold
                Layout.fillWidth: true
            }
            QQC2.Label {
                visible: root.productModes && root.productModes.length > 0
                text: "compatível"
                color: "#3daee9"
                font.pixelSize: 10
            }
        }

        // Botões principais de modo
        RowLayout {
            Layout.fillWidth: true
            spacing: 6

            Repeater {
                model: (root.productModes && root.productModes.length > 0)
                    ? root.productModes
                    : [
                        { name: "Normal", modeKey: "off", defaultcmd: 0 },
                        { name: "ANC", modeKey: "anc", defaultcmd: 1 },
                        { name: "Transparência", modeKey: "transparency", defaultcmd: 3 },
                        { name: "Exterior", modeKey: "outdoor", defaultcmd: 2 }
                    ]

                delegate: QQC2.Button {
                    required property var modelData
                    readonly property bool isModelBacked: root.productModes && root.productModes.length > 0
                    readonly property int targetCmdId: isModelBacked ? (modelData.defaultcmd || modelData.startcmdid) : -1
                    readonly property bool isActive: isModelBacked
                        ? (root.activeMode === modelData)
                        : (root.currentCmdId === modelData.defaultcmd)

                    Layout.fillWidth: true
                    implicitHeight: 36
                    enabled: !root.busy
                    text: root.translateLabel(modelData.name)

                    background: Rectangle {
                        radius: 8
                        color: parent.isActive ? "#3daee9" : (parent.hovered ? "#2e3338" : "#1b1e20")
                        border.width: 1
                        border.color: parent.isActive ? "#3daee9" : "#31363b"
                    }
                    contentItem: QQC2.Label {
                        text: parent.text
                        horizontalAlignment: Text.AlignHCenter
                        verticalAlignment: Text.AlignVCenter
                        color: parent.isActive ? "#1b1e20" : "#eff0f1"
                        font.pixelSize: 12
                        font.weight: Font.DemiBold
                        elide: Text.ElideRight
                    }
                    onClicked: {
                        if (isModelBacked) {
                            root.selectANC(targetCmdId, "");
                        } else {
                            root.selectANC(null, modelData.modeKey);
                        }
                    }
                }
            }
        }

        // Sub-cenários (ex: Adaptativo, Interno, Transporte, Ruidoso, Anti-vento)
        Flow {
            visible: root.activeMode && root.activeMode.items && root.activeMode.items.length > 0
            Layout.fillWidth: true
            spacing: 6

            Repeater {
                model: (root.activeMode && root.activeMode.items) ? root.activeMode.items : []
                delegate: QQC2.Button {
                    required property var modelData
                    readonly property bool isActive: root.activeItem === modelData
                    implicitHeight: 28
                    leftPadding: 10
                    rightPadding: 10
                    enabled: !root.busy
                    text: root.translateLabel(modelData.name)

                    background: Rectangle {
                        radius: 7
                        color: parent.isActive ? "#2a4352" : (parent.hovered ? "#2e3338" : "#1b1e20")
                        border.width: 1
                        border.color: parent.isActive ? "#3daee9" : "#31363b"
                    }
                    contentItem: QQC2.Label {
                        text: parent.text
                        horizontalAlignment: Text.AlignHCenter
                        verticalAlignment: Text.AlignVCenter
                        color: parent.isActive ? "#8be0ff" : "#cfd5db"
                        font.pixelSize: 11
                        font.weight: Font.Medium
                    }
                    onClicked: root.selectANC(modelData.startcmdid, "")
                }
            }
        }

        // Níveis de intensidade (1..N)
        RowLayout {
            visible: root.levelCount > 1
            Layout.fillWidth: true
            spacing: 6

            QQC2.Label {
                text: "Intensidade"
                color: "#9ea7b0"
                font.pixelSize: 11
            }

            Repeater {
                model: root.levelCount
                delegate: QQC2.Button {
                    required property int index
                    readonly property int targetCmdId: root.rangeSource ? (root.rangeSource.startcmdid + index) : -1
                    readonly property bool isActive: root.currentCmdId === targetCmdId
                    implicitWidth: 28
                    implicitHeight: 26
                    enabled: !root.busy
                    text: (index + 1).toString()

                    background: Rectangle {
                        radius: 6
                        color: parent.isActive ? "#3daee9" : (parent.hovered ? "#2e3338" : "#1b1e20")
                        border.width: 1
                        border.color: parent.isActive ? "#3daee9" : "#31363b"
                    }
                    contentItem: QQC2.Label {
                        text: parent.text
                        horizontalAlignment: Text.AlignHCenter
                        verticalAlignment: Text.AlignVCenter
                        color: parent.isActive ? "#1b1e20" : "#eff0f1"
                        font.pixelSize: 11
                        font.weight: Font.Bold
                    }
                    onClicked: root.selectANC(targetCmdId, "")
                }
            }
        }
    }
}
