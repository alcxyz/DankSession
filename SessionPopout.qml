import QtQuick
import QtQuick.Controls
import qs.Common
import qs.Widgets

Column {
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
