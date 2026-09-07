import QtQuick
import QtQuick.Controls
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
                root.statusError = false
            } catch (error) {
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
        SessionPopout {
            sessionStatus: root.sessionStatus
            savedAge: root.savedAge
            autoCapture: root.autoCapture
            autoRestore: root.autoRestore
            captureInterval: root.captureInterval
            statusError: root.statusError
            configureError: root.configureError
            actionError: root.actionError
            actionOutput: root.actionOutput
            busy: root.actionBusy
            onActionRequested: (command, dryRun) => root.runAction(command, dryRun)
        }
    }

    popoutWidth: 360
    // DMS replaces this initial height with the content's measured height.
    popoutHeight: 200

    component SessionPopout: Column {
        id: root
        width: 360
        spacing: Theme.spacingM

        property var sessionStatus: ({saved: false, savedWindows: []})
        property string savedAge: ""
        property bool autoCapture: true
        property bool autoRestore: false
        property int captureInterval: 15
        property bool statusError: false
        property bool configureError: false
        property bool actionError: false
        property bool busy: false
        property string actionOutput: ""
        readonly property var savedWindows: statusError ? [] : (sessionStatus.savedWindows || [])
        readonly property real savedListHeight: savedList.height
        readonly property string serviceText: !sessionStatus.daemonRunning
            ? "Service stopped · Login restore unavailable"
            : ((autoCapture ? "Autosave " + captureInterval + "s" : "Autosave paused")
                + " · Login restore " + (autoRestore ? "on" : "off"))
        signal actionRequested(string command, bool dryRun)

        function workspaceLabel(workspace) {
            if (!workspace) return "Unknown workspace"
            if (workspace.id > 0) return "WS " + workspace.id
            return workspace.name || "Unknown workspace"
        }

        function restoreLabel(mode) {
            if (mode === "relaunch") return "Can reopen"
            if (mode === "excluded") return "Skipped by current rules"
            return "Placement only"
        }

        Row {
            width: parent.width
            StyledText {
                width: parent.width - windowCount.implicitWidth - Theme.spacingS
                text: "Saved session"
                font.pixelSize: Theme.fontSizeLarge
                font.weight: Font.Bold
                color: Theme.surfaceText
            }
            StyledText {
                id: windowCount
                text: root.statusError ? "" : root.savedWindows.length + (root.savedWindows.length === 1 ? " window" : " windows")
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceVariantText
                anchors.verticalCenter: parent.verticalCenter
            }
        }

        StyledText {
            width: parent.width
            text: root.statusError ? "Backend unavailable"
                : (root.sessionStatus.saved ? root.savedAge : "No saved session yet")
            font.pixelSize: Theme.fontSizeSmall
            color: root.statusError ? Theme.error : Theme.surfaceVariantText
            wrapMode: Text.WordWrap
        }

        Flickable {
            id: savedList
            width: parent.width
            height: Math.min(savedRows.implicitHeight, 260)
            visible: root.savedWindows.length > 0
            contentWidth: width
            contentHeight: savedRows.implicitHeight
            clip: true
            boundsBehavior: Flickable.StopAtBounds
            flickableDirection: Flickable.VerticalFlick
            ScrollBar.vertical: DankScrollbar { targetFlickable: savedList }

            Column {
                id: savedRows
                width: parent.width - (savedList.contentHeight > savedList.height ? 12 : 0)
                spacing: Theme.spacingS
                Repeater {
                    model: root.savedWindows
                    delegate: Rectangle {
                        required property var modelData
                        width: savedRows.width
                        height: rowContent.implicitHeight + Theme.spacingS * 2
                        radius: Theme.cornerRadius
                        color: Theme.surfaceContainerHigh
                        Column {
                            id: rowContent
                            x: Theme.spacingS
                            y: Theme.spacingS
                            width: parent.width - Theme.spacingS * 2
                            spacing: 4
                            Row {
                                width: parent.width
                                StyledText {
                                    width: parent.width - workspaceText.width - Theme.spacingS
                                    text: modelData.application
                                    font.pixelSize: Theme.fontSizeMedium
                                    color: Theme.surfaceText
                                    elide: Text.ElideRight
                                }
                                StyledText {
                                    id: workspaceText
                                    width: Math.min(implicitWidth, parent.width * 0.4)
                                    text: root.workspaceLabel(modelData.workspace)
                                    font.pixelSize: Theme.fontSizeSmall
                                    color: Theme.surfaceVariantText
                                    elide: Text.ElideRight
                                }
                            }
                            StyledText {
                                width: parent.width
                                text: modelData.width + " × " + modelData.height + " · " + root.restoreLabel(modelData.restore)
                                font.pixelSize: Theme.fontSizeSmall
                                color: Theme.surfaceVariantText
                                wrapMode: Text.WordWrap
                            }
                        }
                    }
                }
            }
        }

        StyledText {
            width: parent.width
            visible: !root.statusError && root.sessionStatus.saved && root.savedWindows.length === 0
            text: "No windows in this snapshot."
            font.pixelSize: Theme.fontSizeSmall
            color: Theme.surfaceVariantText
            wrapMode: Text.WordWrap
        }

        StyledText {
            width: parent.width
            text: root.serviceText
            font.pixelSize: Theme.fontSizeSmall
            color: Theme.surfaceVariantText
            wrapMode: Text.WordWrap
        }

        Row {
            width: parent.width
            spacing: Theme.spacingS
            DankButton {
                width: (parent.width - Theme.spacingS * 2) / 3
                buttonHeight: 36
                text: "Save now"
                enabled: !root.busy
                onClicked: root.actionRequested("capture", false)
            }
            DankButton {
                width: (parent.width - Theme.spacingS * 2) / 3
                buttonHeight: 36
                text: "Preview"
                enabled: !root.busy && !root.statusError && root.savedWindows.length > 0
                onClicked: root.actionRequested("restore", true)
            }
            DankButton {
                width: (parent.width - Theme.spacingS * 2) / 3
                buttonHeight: 36
                text: "Restore"
                enabled: !root.busy && !root.statusError && root.savedWindows.length > 0
                onClicked: root.actionRequested("restore", false)
            }
        }

        StyledText {
            width: parent.width
            visible: root.configureError
            text: "Unable to apply settings to the backend."
            font.pixelSize: Theme.fontSizeSmall
            color: Theme.error
            wrapMode: Text.WordWrap
        }

        StyledText {
            width: parent.width
            visible: root.actionOutput !== ""
            text: root.actionOutput
            font.pixelSize: Theme.fontSizeSmall
            color: root.actionError ? Theme.error : Theme.surfaceVariantText
            wrapMode: Text.WrapAnywhere
            maximumLineCount: 4
            elide: Text.ElideRight
        }
    }
}
