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
    property bool autoCapture: true
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
    property string _actionCommand: ""
    property bool _actionDryRun: false
    property string savedAge: ""
    property string _configureSignature: ""
    property bool _settingsReady: false
    readonly property bool actionBusy: actionProcess.running || restoreStart.running

    // DMS may retain plugin settings in memory across a Home Manager update.
    // Refresh only this plugin's persisted preferences before configuring the
    // backend, including when the widget is reloaded without restarting DMS.
    FileView {
        id: persistedSettings
        path: SettingsData.pluginSettingsPath
        blockLoading: true
        watchChanges: true
        onFileChanged: reload()
        onLoaded: {
            try {
                var saved = JSON.parse(text())[root.pluginId] || {}
                var keys = ["autoCapture", "autoRestore", "captureUnconfigured", "captureTitles", "captureInterval", "restoreTimeout"]
                for (var key of keys) {
                    if (saved[key] !== undefined && root.pluginService
                            && root.pluginService.loadPluginData(root.pluginId, key, undefined) !== saved[key]) {
                        root.pluginService.savePluginData(root.pluginId, key, saved[key])
                    }
                }
                root._settingsReady = true
                root.loadSettings()
            } catch (error) {
                root.configureError = true
            }
        }
    }

    function loadSettings() {
        if (!_settingsReady) return
        if (!pluginService || !pluginService.loadPluginData) return
        autoCapture = pluginService.loadPluginData(pluginId, "autoCapture", true) !== false
        autoRestore = pluginService.loadPluginData(pluginId, "autoRestore", false) === true
        captureUnconfigured = pluginService.loadPluginData(pluginId, "captureUnconfigured", true) !== false
        captureTitles = pluginService.loadPluginData(pluginId, "captureTitles", false) === true
        captureInterval = pluginService.loadPluginData(pluginId, "captureInterval", 15) || 15
        restoreTimeout = pluginService.loadPluginData(pluginId, "restoreTimeout", 20) || 20
        syncConfiguration()
    }

    function syncConfiguration() {
        var signature = [autoCapture, autoRestore, captureUnconfigured, captureTitles, captureInterval, restoreTimeout].join("|")
        if (signature === _configureSignature || configureProcess.running) return
        _configureSignature = signature
        configureProcess.command = [
            "danksession", "configure",
            "--auto-capture=" + autoCapture,
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
        if (actionBusy) return
        _actionOutput = ""
        _actionCommand = command
        _actionDryRun = dryRun === true
        actionError = false
        actionProcess.command = dryRun ? ["danksession", command, "--dry-run"] : ["danksession", command]
        if (command === "restore" && !dryRun) {
            // The clicked layer-shell popout owns keyboard focus. Release it
            // before Hyprland's focus-dependent layout dispatchers run.
            closePopout()
            restoreStart.restart()
            return
        }
        actionProcess.running = true
    }

    function updateSavedAge() {
        if (!sessionStatus.savedAt) { savedAge = ""; return }
        var seconds = Math.max(0, Math.floor((Date.now() - new Date(sessionStatus.savedAt).getTime()) / 1000))
        savedAge = seconds < 60 ? "Saved " + seconds + " seconds ago"
            : (seconds < 3600 ? "Saved " + Math.floor(seconds / 60) + " minutes ago"
                : "Saved " + new Date(sessionStatus.savedAt).toLocaleString())
    }

    Timer {
        id: restoreStart
        interval: 150
        onTriggered: actionProcess.running = true
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
            root.updateSavedAge()
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
                root.updateSavedAge()
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
        stderr: StdioCollector { id: actionStderr }
        onExited: (exitCode, exitStatus) => {
            try {
                var result = JSON.parse(root._actionOutput.trim())
                if (result.error) throw new Error(result.error)
                if (exitCode !== 0) throw new Error("The operation did not complete.")
                root.actionError = false
                if (root._actionCommand === "capture") {
                    root.actionOutput = "Saved " + result.windows + " windows."
                } else {
                    root.actionOutput = (root._actionDryRun ? "Preview: " : "Restore finished: ")
                        + result.matched + " windows matched, " + result.missing + " unavailable."
                    if (result.launched) root.actionOutput += " Reopened " + result.launched + " applications."
                    if (root._actionDryRun) root.actionOutput += " Nothing has been changed."
                }
            } catch (error) {
                root.actionError = true
                root.actionOutput = error.message || "The operation failed. Check that the backend is available."
                if (!root._actionOutput.trim() && actionStderr.text.trim()) root.actionOutput = actionStderr.text.trim()
            }
            root.refreshStatus()
        }
    }

    // Keep the bar quiet; counts, timestamps, and actions belong in the popout.
    horizontalBarPill: compactBarIcon
    verticalBarPill: compactBarIcon

    Component {
        id: compactBarIcon
        DankIcon {
            name: "restore_page"
            size: root.iconSize
            color: root.hasError ? Theme.error
                : (root.sessionStatus.daemonRunning && root.autoCapture
                    ? Theme.primary : Theme.surfaceVariantText)
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
                    text: root.sessionStatus.managed + " windows with launch rules · " + root.savedAge
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
                        opacity: root.actionBusy ? 0.5 : 1
                        StyledText { text: "Save now"; color: "#ffffff"; anchors.centerIn: parent }
                        MouseArea {
                            id: saveMouse
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            enabled: !root.actionBusy
                            onClicked: root.runAction("capture")
                        }
                    }

                    Rectangle {
                        width: 96
                        height: 36
                        radius: Theme.cornerRadius
                        color: restoreMouse.containsMouse ? Theme.withAlpha(Theme.primary, 0.25) : Theme.surfaceContainerHigh
                        opacity: root.actionBusy || !root.sessionStatus.saved ? 0.5 : 1
                        StyledText { text: "Restore"; color: Theme.surfaceText; anchors.centerIn: parent }
                        MouseArea {
                            id: restoreMouse
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            enabled: !root.actionBusy && root.sessionStatus.saved
                            onClicked: root.runAction("restore")
                        }
                    }

                    Rectangle {
                        width: 96
                        height: 36
                        radius: Theme.cornerRadius
                        color: Theme.surfaceContainerHigh
                        opacity: root.actionBusy || !root.sessionStatus.saved ? 0.5 : 1
                        StyledText { text: "Preview"; color: Theme.surfaceText; anchors.centerIn: parent }
                        MouseArea {
                            anchors.fill: parent
                            cursorShape: Qt.PointingHandCursor
                            enabled: !root.actionBusy && root.sessionStatus.saved
                            onClicked: root.runAction("restore", true)
                        }
                    }
                }

                StyledText {
                    width: parent.width
                    visible: root.actionOutput !== ""
                    text: root.actionOutput
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
                        : ((!root.autoCapture ? "Automatic saving is paused. "
                            : "Saving changes automatically, with a safety save every " + root.captureInterval + " seconds. ")
                            + (root.autoRestore ? "Restore after login is enabled." : "Restore after login is disabled."))
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
