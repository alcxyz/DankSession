import QtQuick
import Quickshell
import Quickshell.Io
import qs.Common
import qs.Widgets
import qs.Modules.Plugins

PluginComponent {
    id: root
    pluginId: "dankSession"

    property bool autoRestore: false
    property bool captureUnconfigured: true
    property bool captureTitles: false
    property int captureInterval: 15
    property int restoreTimeout: 20
    property var sessionStatus: ({saved: false, windows: 0, managed: 0, workspaces: 0})
    property string statusText: "No snapshot"
    property string actionOutput: ""
    property bool statusError: false
    property bool actionError: false
    property bool configureError: false
    readonly property bool hasError: statusError || actionError || configureError
    property string _statusOutput: ""
    property string _actionOutput: ""
    property string _configureSignature: ""

    function loadSettings() {
        if (!pluginService || !pluginService.loadPluginData) return
        autoRestore = pluginService.loadPluginData(pluginId, "autoRestore", false) === true
        captureUnconfigured = pluginService.loadPluginData(pluginId, "captureUnconfigured", true) !== false
        captureTitles = pluginService.loadPluginData(pluginId, "captureTitles", false) === true
        captureInterval = pluginService.loadPluginData(pluginId, "captureInterval", 15) || 15
        restoreTimeout = pluginService.loadPluginData(pluginId, "restoreTimeout", 20) || 20
        syncConfiguration()
    }

    function syncConfiguration() {
        var signature = [autoRestore, captureUnconfigured, captureTitles, captureInterval, restoreTimeout].join("|")
        if (signature === _configureSignature || configureProcess.running) return
        _configureSignature = signature
        configureProcess.command = [
            "danksession", "configure",
            "--auto-restore=" + autoRestore,
            "--capture-unconfigured=" + captureUnconfigured,
            "--capture-titles=" + captureTitles,
            "--capture-interval=" + captureInterval,
            "--restore-timeout=" + restoreTimeout
        ]
        configureProcess.running = true
    }

    function refreshStatus() {
        if (statusProcess.running) return
        _statusOutput = ""
        statusProcess.running = true
    }

    function runAction(command, dryRun) {
        if (actionProcess.running) return
        _actionOutput = ""
        actionError = false
        actionProcess.command = dryRun ? ["danksession", command, "--dry-run"] : ["danksession", command]
        actionProcess.running = true
    }

    Component.onCompleted: {
        loadSettings()
        refreshStatus()
    }

    Timer {
        interval: 3000
        running: true
        repeat: true
        onTriggered: {
            root.loadSettings()
            root.refreshStatus()
        }
    }

    Process {
        id: configureProcess
        running: false
        stdout: StdioCollector {}
        onExited: (exitCode, exitStatus) => {
            root.configureError = exitCode !== 0
            if (exitCode !== 0) root._configureSignature = ""
        }
    }

    Process {
        id: statusProcess
        running: false
        command: ["danksession", "status"]
        stdout: SplitParser { onRead: data => { root._statusOutput += data + "\n" } }
        onExited: (exitCode, exitStatus) => {
            try {
                var parsed = JSON.parse(root._statusOutput.trim())
                if (parsed.error) throw new Error(parsed.error)
                root.sessionStatus = parsed
                root.statusText = parsed.saved
                    ? (parsed.windows + " windows across " + parsed.workspaces + " workspaces")
                    : "No snapshot"
                root.statusError = false
            } catch (error) {
                root.statusText = "Backend unavailable"
                root.statusError = true
            }
        }
    }

    Process {
        id: actionProcess
        running: false
        stdout: SplitParser { onRead: data => { root._actionOutput += data + "\n" } }
        onExited: (exitCode, exitStatus) => {
            root.actionOutput = root._actionOutput.trim()
            root.actionError = exitCode !== 0
            root.refreshStatus()
        }
    }

    horizontalBarPill: Component {
        Row {
            spacing: Theme.spacingS
            anchors.verticalCenter: parent.verticalCenter

            DankIcon {
                name: "restore_page"
                size: 16
                color: root.hasError ? Theme.error : Theme.primary
                anchors.verticalCenter: parent.verticalCenter
            }

            StyledText {
                text: root.sessionStatus.saved ? (root.sessionStatus.windows + " saved") : "Session"
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceText
                anchors.verticalCenter: parent.verticalCenter
            }
        }
    }

    verticalBarPill: Component {
        Column {
            spacing: 1
            anchors.horizontalCenter: parent.horizontalCenter

            StyledText {
                text: root.sessionStatus.saved ? root.sessionStatus.windows : "–"
                font.pixelSize: Theme.fontSizeSmall
                color: root.hasError ? Theme.error : Theme.surfaceText
                anchors.horizontalCenter: parent.horizontalCenter
            }

            StyledText {
                text: "sess"
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceVariantText
                anchors.horizontalCenter: parent.horizontalCenter
            }
        }
    }

    popoutContent: Component {
        Item {
            implicitWidth: root.popoutWidth
            implicitHeight: root.popoutHeight

            Column {
                anchors.fill: parent
                spacing: Theme.spacingL

                StyledText {
                    text: "Session"
                    font.pixelSize: Theme.fontSizeXLarge
                    font.weight: Font.Bold
                    color: Theme.surfaceText
                }

                StyledText {
                    width: parent.width
                    text: root.statusText
                    font.pixelSize: Theme.fontSizeMedium
                    color: root.hasError ? Theme.error : Theme.surfaceVariantText
                    wrapMode: Text.WordWrap
                }

                StyledText {
                    width: parent.width
                    visible: root.sessionStatus.saved
                    text: root.sessionStatus.managed + " launch-managed · "
                        + (root.sessionStatus.savedAt ? new Date(root.sessionStatus.savedAt).toLocaleString() : "")
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.surfaceVariantText
                    wrapMode: Text.WordWrap
                }

                Row {
                    spacing: Theme.spacingM

                    Rectangle {
                        width: 96
                        height: 36
                        radius: Theme.cornerRadius
                        color: saveMouse.containsMouse ? Theme.withAlpha(Theme.primary, 0.3) : Theme.primary
                        opacity: actionProcess.running ? 0.5 : 1
                        StyledText { text: "Save now"; color: "#ffffff"; anchors.centerIn: parent }
                        MouseArea {
                            id: saveMouse
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            enabled: !actionProcess.running
                            onClicked: root.runAction("capture")
                        }
                    }

                    Rectangle {
                        width: 96
                        height: 36
                        radius: Theme.cornerRadius
                        color: restoreMouse.containsMouse ? Theme.withAlpha(Theme.primary, 0.25) : Theme.surfaceContainerHigh
                        opacity: actionProcess.running || !root.sessionStatus.saved ? 0.5 : 1
                        StyledText { text: "Restore"; color: Theme.surfaceText; anchors.centerIn: parent }
                        MouseArea {
                            id: restoreMouse
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            enabled: !actionProcess.running && root.sessionStatus.saved
                            onClicked: root.runAction("restore")
                        }
                    }

                    Rectangle {
                        width: 96
                        height: 36
                        radius: Theme.cornerRadius
                        color: Theme.surfaceContainerHigh
                        opacity: actionProcess.running || !root.sessionStatus.saved ? 0.5 : 1
                        StyledText { text: "Preview"; color: Theme.surfaceText; anchors.centerIn: parent }
                        MouseArea {
                            anchors.fill: parent
                            cursorShape: Qt.PointingHandCursor
                            enabled: !actionProcess.running && root.sessionStatus.saved
                            onClicked: root.runAction("restore", true)
                        }
                    }
                }

                StyledText {
                    width: parent.width
                    visible: root.actionOutput !== ""
                    text: root.actionOutput
                    font.family: "monospace"
                    font.pixelSize: Theme.fontSizeSmall
                    color: root.hasError ? Theme.error : Theme.surfaceVariantText
                    wrapMode: Text.WrapAnywhere
                    maximumLineCount: 8
                    elide: Text.ElideRight
                }

                StyledText {
                    width: parent.width
                    text: !root.sessionStatus.daemonRunning
                        ? "Capture daemon is stopped. Saving and restoring are manual; login restoration also requires daemon startup to be enabled in your system configuration."
                        : (root.autoRestore
                            ? "Capture daemon is running. Automatic restoration is enabled for the next graphical login."
                            : "Capture daemon is running. Automatic restoration is disabled.")
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.surfaceVariantText
                    wrapMode: Text.WordWrap
                }
            }
        }
    }

    popoutWidth: 360
    popoutHeight: 440
}
