import QtQuick
import QtQuick.Layouts
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami

Rectangle {
    id: root
    property string label: ""
    property int level: -1
    property bool charging: false
    property bool hasReading: level >= 0

    radius: 10
    color: "#232629"
    border.width: 1
    border.color: "#31363b"
    implicitHeight: 74

    function levelColor(val, isCharging) {
        if (!hasReading) return "#4d5359";
        if (isCharging) return "#3daee9";
        if (val <= 15) return "#da4453";
        if (val <= 25) return "#f39c12";
        return "#27ae60";
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 10
        spacing: 6

        RowLayout {
            Layout.fillWidth: true
            QQC2.Label {
                text: root.label
                color: "#9ea7b0"
                font.pixelSize: 10
                font.weight: Font.DemiBold
                Layout.fillWidth: true
            }
            QQC2.Label {
                visible: root.charging
                text: "⚡"
                color: "#3daee9"
                font.pixelSize: 11
            }
        }

        QQC2.Label {
            text: root.hasReading ? (root.level + "%") : "--%"
            color: "#eff0f1"
            font.pixelSize: 18
            font.weight: Font.Bold
        }

        Rectangle {
            Layout.fillWidth: true
            height: 5
            radius: 3
            color: "#171a1c"

            Rectangle {
                height: parent.height
                radius: 3
                width: root.hasReading ? Math.max(0, Math.min(1, root.level / 100)) * parent.width : 0
                color: root.levelColor(root.level, root.charging)

                Behavior on width {
                    NumberAnimation { duration: 250; easing.type: Easing.OutCubic }
                }
            }
        }
    }
}
