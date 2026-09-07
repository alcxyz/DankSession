import QtQuick
import Quickshell.Io
import qs.Common
import qs.Widgets
import qs.Modules.Plugins

PluginSettings {
    id: root
    pluginId: "dankSession"
    property bool daemonRunning: false
    property bool statusKnown: false
    property string statusOutput: ""
    property string configurationError: ""
    property bool configurationPending: false

    // Apply edits even when the widget is not placed on a bar. Debounce slider
    // changes, and let the backend merge preferences under its config lock.
    onSettingChanged: {
        configurationPending = true
        configureTimer.restart()
    }

    Timer {
        id: configureTimer
        interval: 250
        onTriggered: {
            if (configureProcess.running) { restart(); return }
            root.configurationPending = false
            configureProcess.command = [
                "danksession", "configure",
                "--auto-capture=" + (root.loadValue("autoCapture", true) !== false),
                "--auto-restore=" + (root.loadValue("autoRestore", false) === true),
                "--capture-unconfigured=" + (root.loadValue("captureUnconfigured", true) !== false),
                "--capture-titles=" + (root.loadValue("captureTitles", false) === true),
                "--capture-interval=" + Math.round(root.loadValue("captureInterval", 15)),
                "--restore-timeout=" + Math.round(root.loadValue("restoreTimeout", 20))
            ]
            configureProcess.running = true
        }
    }

    Process {
        id: configureProcess
        stdout: StdioCollector { id: configureOutput }
        onExited: (exitCode, exitStatus) => {
            root.configurationError = ""
            if (exitCode !== 0) {
                try {
                    root.configurationError = JSON.parse(configureOutput.text).error || "Unable to apply settings."
                } catch (error) {
                    root.configurationError = "Unable to apply settings to the backend."
                }
                if (root.configurationError.indexOf("operation is in progress") !== -1) configureTimer.restart()
            }
            if (root.configurationPending) configureTimer.restart()
        }
    }

    Process {
        id: serviceStatus
        command: ["danksession", "status"]
        running: true
        stdout: SplitParser { onRead: data => { root.statusOutput += data + "\n" } }
        onExited: (exitCode, exitStatus) => {
            try {
                var result = JSON.parse(root.statusOutput)
                root.statusKnown = exitCode === 0 && !result.error
                root.daemonRunning = result.daemonRunning === true
            } catch (error) {
                root.statusKnown = false
            }
        }
    }

    Timer {
        interval: 5000
        repeat: true
        running: root.visible
        onTriggered: {
            if (serviceStatus.running) return
            root.statusOutput = ""
            serviceStatus.running = true
        }
    }

    StyledText {
        text: "Session restoration"
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Font.Bold
        color: Theme.surfaceText
    }

    StyledText {
        width: parent.width
        visible: root.configurationError !== ""
        text: root.configurationError
        color: Theme.error
        wrapMode: Text.WordWrap
    }

    ToggleSetting {
        settingKey: "autoCapture"
        label: "Automatic saving"
        description: "Save changes in the background. Requires the capture service to be running; Save now remains available when paused."
        defaultValue: true
    }

    StyledText {
        width: parent.width
        visible: !root.statusKnown || !root.daemonRunning
        text: root.statusKnown
            ? "The background service is stopped. Automatic saving and login restoration are unavailable until it is enabled in your system configuration. Manual Save now and Restore still work."
            : "Unable to check the background service. Automatic saving may be unavailable."
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.error
        wrapMode: Text.WordWrap
    }

    ToggleSetting {
        settingKey: "autoRestore"
        label: "Restore after login"
        description: "Restore the last saved layout once after login. Only applications with an explicit launch rule can reopen."
        defaultValue: false
    }

    ToggleSetting {
        settingKey: "captureUnconfigured"
        label: "Capture unconfigured applications"
        description: "Remember placement for running applications without relaunching them"
        defaultValue: true
    }

    ToggleSetting {
        settingKey: "captureTitles"
        label: "Capture window titles"
        description: "Improves matching of multiple windows, but stores titles in the local snapshot"
        defaultValue: false
    }

    SliderSetting {
        settingKey: "captureInterval"
        label: "Save frequency (seconds)"
        description: "Periodic safety saves while automatic saving is enabled; window changes are saved sooner."
        minimum: 5
        maximum: 120
        defaultValue: 15
    }

    SliderSetting {
        settingKey: "restoreTimeout"
        label: "Restore timeout (seconds)"
        description: "How long to wait for configured applications to create their windows"
        minimum: 5
        maximum: 120
        defaultValue: 20
    }

    StyledText {
        width: parent.width
        text: "Application launch rules live in ~/.config/danksession/config.json. Window titles are not saved by default. Changing settings never launches or restarts applications."
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    ExclusionEditor {
        width: parent.width
    }
}
