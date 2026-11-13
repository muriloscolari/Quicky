import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.kcmutils as KCM

KCM.SimpleKCM {
    property alias cfg_daemonUrl: daemonUrlField.text
    property alias cfg_pollIntervalMs: pollSpinBox.value

    Kirigami.FormLayout {
        QQC2.TextField {
            id: daemonUrlField
            Kirigami.FormData.label: "URL do daemon:"
            placeholderText: "http://127.0.0.1:47892"
        }

        QQC2.SpinBox {
            id: pollSpinBox
            Kirigami.FormData.label: "Intervalo de atualização (ms):"
            from: 1500
            to: 30000
            stepSize: 500
        }
    }
}
