import QtQuick
import QtQuick.Controls
import Quickshell.Io
import qs.Common
import qs.Widgets

Column {
    id: root
    spacing: Theme.spacingM

    property string revision: ""
    property var rules: []
    property var applications: []
    property string message: ""
    property bool messageError: false
    property string operation: ""
    property string requestBody: ""
    property string responseBody: ""
    property string errorBody: ""
    property bool editorOpen: false
    property bool pickerOpen: false
    property bool advanced: false
    property int editIndex: -1
    property int removeIndex: -1
    property bool draftDisabled: false
    property string literalField: "initialClass"
    property string previewBody: ""
    property string previewErrorBody: ""
    property string previewRequest: ""
    property string previewSignature: ""
    property string previewError: ""
    property var previewApplications: []
    property int previewWindows: 0
    readonly property bool busy: commandProcess.running
    readonly property string draftSignature: JSON.stringify(draftMatch())
    readonly property bool previewReady: editorOpen && previewSignature === draftSignature && !previewError
    readonly property var filteredApplications: applications.filter(app =>
        ((app.initialClass || "") + " " + (app.class || "")).toLowerCase().indexOf(appSearch.text.toLowerCase()) !== -1)

    function literalPattern(value) {
        return "^" + value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "$"
    }

    function draftMatch() {
        var match = {disabled: draftDisabled}
        if (advanced) {
            if (initialClassInput.text) match.initialClass = initialClassInput.text
            if (classInput.text) match.class = classInput.text
            if (titleInput.text) match.title = titleInput.text
        } else if (appInput.text.trim()) {
            match[literalField] = literalPattern(appInput.text.trim())
        }
        return match
    }

    function description(match) {
        var parts = []
        if (match.initialClass) parts.push("Application: " + match.initialClass)
        if (match.class) parts.push("Class: " + match.class)
        if (match.title) parts.push("Title: " + match.title)
        return parts.join(" · ")
    }

    function startCommand(kind, body) {
        if (busy) return
        operation = kind
        requestBody = body ? JSON.stringify(body) : ""
        responseBody = ""
        errorBody = ""
        commandProcess.command = ["danksession", "exclusions", kind]
        commandProcess.stdinEnabled = !!body
        commandProcess.running = true
    }

    function refresh() {
        startCommand("list")
    }

    function beginEdit(rule, app) {
        pickerOpen = false
        editIndex = rule ? rule.index : -1
        removeIndex = -1
        draftDisabled = rule ? rule.disabled === true : false
        advanced = !!rule
        literalField = app && !app.initialClass ? "class" : "initialClass"
        appInput.text = app ? (app.initialClass || app.class) : ""
        initialClassInput.text = rule ? (rule.initialClass || "") : ""
        classInput.text = rule ? (rule.class || "") : ""
        titleInput.text = rule ? (rule.title || "") : ""
        previewSignature = ""
        previewError = ""
        editorOpen = true
        previewTimer.restart()
    }

    function showAdvanced() {
        var match = draftMatch()
        initialClassInput.text = match.initialClass || ""
        classInput.text = match.class || ""
        titleInput.text = match.title || ""
        advanced = true
    }

    function updateRule(action, index, match) {
        message = ""
        var body = {action: action, revision: revision}
        if (index >= 0) body.index = index
        if (match) body.match = match
        startCommand("update", body)
    }

    function toggleRule(rule) {
        var match = {disabled: !rule.disabled}
        if (rule.initialClass) match.initialClass = rule.initialClass
        if (rule.class) match.class = rule.class
        if (rule.title) match.title = rule.title
        updateRule("update", rule.index, match)
    }

    onDraftSignatureChanged: {
        previewSignature = ""
        previewError = ""
        if (editorOpen) previewTimer.restart()
    }
    Component.onCompleted: refresh()

    Timer {
        interval: 3000
        running: root.editorOpen && !root.busy
        repeat: true
        onTriggered: previewTimer.restart()
    }

    Timer {
        id: previewTimer
        interval: 300
        onTriggered: {
            if (!root.editorOpen) return
            if (previewProcess.running) {
                restart()
                return
            }
            var match = root.draftMatch()
            if (!match.initialClass && !match.class && !match.title) {
                root.previewError = "Enter an application identifier or a matching pattern."
                return
            }
            root.previewBody = ""
            root.previewErrorBody = ""
            root.previewRequest = root.draftSignature
            previewProcess.stdinEnabled = true
            previewProcess.running = true
        }
    }

    Process {
        id: commandProcess
        onStarted: {
            if (!root.requestBody) return
            write(root.requestBody + "\n")
            // Quickshell sends EOF when stdinEnabled is disabled.
            stdinEnabled = false
        }
        stdout: SplitParser { onRead: data => { root.responseBody += data + "\n" } }
        stderr: SplitParser { onRead: data => { root.errorBody += data + "\n" } }
        onExited: (exitCode, exitStatus) => {
            try {
                var result = JSON.parse(root.responseBody)
                if (exitCode !== 0 || result.error) throw new Error(result.error || root.errorBody.trim() || "The backend could not complete this request.")
                if (root.operation === "list" && root.revision && root.revision !== result.revision
                        && (root.editorOpen || root.removeIndex >= 0)) {
                    root.editorOpen = false
                    root.removeIndex = -1
                    root.message = "Exclusions changed elsewhere. Select the rule again before editing."
                    root.messageError = true
                }
                root.revision = result.revision
                root.rules = result.rules || []
                root.applications = result.applications || []
                if (root.operation === "update") {
                    root.editorOpen = false
                    root.removeIndex = -1
                    root.message = "Exclusions updated. No applications were launched or restarted."
                    root.messageError = false
                }
            } catch (error) {
                root.message = root.responseBody.trim() ? String(error).replace(/^Error: /, "") : "Unable to read exclusions. Check that the DankSession backend is installed and available."
                root.messageError = true
                if (root.message.indexOf("exclusions changed") !== -1) {
                    // Indices may now refer to different rules. Reload, and ask
                    // the user to select the intended rule again.
                    root.editorOpen = false
                    root.removeIndex = -1
                    root.message = "Exclusions changed elsewhere. The list has been refreshed; select the rule again."
                    Qt.callLater(root.refresh)
                }
            }
        }
    }

    Process {
        id: previewProcess
        command: ["danksession", "exclusions", "preview"]
        onStarted: {
            write(root.previewRequest + "\n")
            stdinEnabled = false
        }
        stdout: SplitParser { onRead: data => { root.previewBody += data + "\n" } }
        stderr: SplitParser { onRead: data => { root.previewErrorBody += data + "\n" } }
        onExited: (exitCode, exitStatus) => {
            if (!root.editorOpen || root.previewRequest !== root.draftSignature) return
            try {
                var result = JSON.parse(root.previewBody)
                if (exitCode !== 0 || result.error) throw new Error(result.error || root.previewErrorBody.trim() || "Could not check this match.")
                root.previewWindows = result.windows
                root.previewApplications = result.applications || []
                root.previewError = ""
                root.previewSignature = root.previewRequest
            } catch (error) {
                root.previewSignature = ""
                root.previewError = root.previewBody.trim() ? String(error).replace(/^Error: /, "") : "Unable to check open windows. Try refreshing the list."
            }
        }
    }

    StyledText {
        text: "Application exclusions"
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Font.Bold
        color: Theme.surfaceText
    }

    StyledText {
        width: parent.width
        text: "Excluded applications are not saved, reopened, or repositioned—even when restoring an older snapshot. Existing windows stay untouched."
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    StyledText {
        width: parent.width
        visible: root.message !== ""
        text: root.message
        color: root.messageError ? Theme.error : Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    Flow {
        width: parent.width
        spacing: Theme.spacingS
        DankButton { text: "Add manually"; iconName: "add"; enabled: !root.busy && !!root.revision; onClicked: root.beginEdit(null, null) }
        DankButton {
            text: root.pickerOpen ? "Hide open applications" : "Choose open application…"
            iconName: "apps"
            enabled: !root.busy && !!root.revision
            onClicked: {
                root.pickerOpen = !root.pickerOpen
                if (root.pickerOpen) {
                    root.editorOpen = false
                    root.refresh()
                }
            }
        }
        DankButton { text: root.busy ? "Loading…" : "Refresh"; iconName: "refresh"; enabled: !root.busy; onClicked: root.refresh() }
    }

    Column {
        width: parent.width
        spacing: Theme.spacingS
        visible: root.pickerOpen
        StyledText {
            text: "From currently open applications"
            font.weight: Font.Medium
            color: Theme.surfaceText
        }
        DankTextField {
            id: appSearch
            width: parent.width
            placeholderText: "Search application identifiers"
            leftIconName: "search"
            showClearButton: true
            onTextChanged: appFlickable.contentY = 0
        }
        Flickable {
            id: appFlickable
            width: parent.width
            height: Math.min(240, appList.implicitHeight)
            contentWidth: width
            contentHeight: appList.implicitHeight
            clip: true
            boundsBehavior: Flickable.StopAtBounds
            flickableDirection: Flickable.VerticalFlick
            ScrollBar.vertical: DankScrollbar {}
            Column {
                id: appList
                width: appFlickable.width - 12
                spacing: Theme.spacingS
                Repeater {
                    model: root.filteredApplications
                    delegate: Row {
                        required property var modelData
                        width: appList.width
                        spacing: Theme.spacingS
                        StyledText {
                            width: Math.max(80, parent.width - chooseButton.width - Theme.spacingS)
                            anchors.verticalCenter: parent.verticalCenter
                            text: (modelData.initialClass || modelData.class) + " · " + modelData.windows + " window(s) · WS " + modelData.workspaces.join(", ")
                            color: Theme.surfaceText
                            wrapMode: Text.WrapAnywhere
                        }
                        DankButton {
                            id: chooseButton
                            text: "Exclude…"
                            enabled: !root.busy && !!root.revision
                            onClicked: root.beginEdit(null, modelData)
                        }
                    }
                }
            }
        }
        StyledText {
            width: parent.width
            visible: root.filteredApplications.length === 0
            text: root.busy ? "Reading open applications…" : "No matching open applications. You can still add an identifier manually."
            color: Theme.surfaceVariantText
            wrapMode: Text.WordWrap
        }
    }

    Rectangle {
        width: parent.width
        height: editorColumn.implicitHeight + Theme.spacingM * 2
        visible: root.editorOpen
        radius: Theme.cornerRadius
        color: Theme.surfaceContainerHigh
        Column {
            id: editorColumn
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: Theme.spacingM
            spacing: Theme.spacingM
            enabled: !root.busy

            StyledText {
                text: root.editIndex < 0 ? "New exclusion" : "Edit exclusion"
                font.weight: Font.Bold
                color: Theme.surfaceText
            }
            DankTextField {
                id: appInput
                width: parent.width
                visible: !root.advanced
                labelText: "Application identifier (exact match)"
                placeholderText: "For example: thunderbird"
            }
            StyledText {
                width: parent.width
                visible: !root.advanced
                text: "Matches the " + (root.literalField === "initialClass" ? "initial application class" : "window class") + " exactly, not a changing window title. Special characters are treated literally."
                color: Theme.surfaceVariantText
                font.pixelSize: Theme.fontSizeSmall
                wrapMode: Text.WordWrap
            }
            DankButton {
                visible: !root.advanced
                text: "Advanced matching…"
                onClicked: root.showAdvanced()
            }
            Column {
                width: parent.width
                spacing: Theme.spacingM
                visible: root.advanced
                StyledText {
                    width: parent.width
                    text: "Advanced: Go regular expressions. All nonempty fields must match. Leave title empty to exclude the whole application."
                    color: Theme.surfaceVariantText
                    font.pixelSize: Theme.fontSizeSmall
                    wrapMode: Text.WordWrap
                }
                DankTextField { id: initialClassInput; width: parent.width; labelText: "Initial application class pattern"; placeholderText: "^thunderbird$" }
                DankTextField { id: classInput; width: parent.width; labelText: "Window class pattern (optional)" }
                DankTextField { id: titleInput; width: parent.width; labelText: "Window title pattern (optional)" }
                StyledText {
                    width: parent.width
                    visible: titleInput.text !== ""
                    text: "Older snapshots may not contain titles. For safety, this rule also skips saved windows with unknown titles that match its class fields. A title-only rule skips all windows whose saved title is unknown."
                    color: Theme.error
                    font.pixelSize: Theme.fontSizeSmall
                    wrapMode: Text.WordWrap
                }
            }
            StyledText {
                width: parent.width
                text: root.previewError || (root.previewReady
                    ? (root.previewWindows === 0 ? "No open windows match. This exclusion can still apply when the application opens later." : "Matches " + root.previewWindows + " currently open window(s):")
                    : "Checking open windows…")
                color: root.previewError ? Theme.error : Theme.surfaceVariantText
                wrapMode: Text.WordWrap
            }
            Repeater {
                model: root.previewReady ? root.previewApplications : []
                delegate: StyledText {
                    required property var modelData
                    width: editorColumn.width
                    text: (modelData.initialClass || modelData.class) + " · " + modelData.windows + " window(s) · WS " + modelData.workspaces.join(", ")
                    color: Theme.surfaceText
                    wrapMode: Text.WrapAnywhere
                }
            }
            StyledText {
                visible: root.draftDisabled
                text: "This rule is disabled. Saving edits will keep it disabled."
                width: parent.width
                color: Theme.surfaceVariantText
                wrapMode: Text.WordWrap
            }
            Flow {
                width: parent.width
                spacing: Theme.spacingS
                DankButton {
                    text: root.editIndex < 0 ? "Confirm exclusion" : "Save changes"
                    enabled: root.previewReady && !root.busy
                    onClicked: root.updateRule(root.editIndex < 0 ? "add" : "update", root.editIndex, root.draftMatch())
                }
                DankButton { text: "Cancel"; onClicked: root.editorOpen = false }
            }
        }
    }

    StyledText {
        text: "Saved exclusions (" + root.rules.length + ")"
        font.weight: Font.Medium
        color: Theme.surfaceText
    }

    Repeater {
        model: root.rules
        delegate: Column {
            required property var modelData
            width: root.width
            spacing: Theme.spacingS
            StyledText {
                width: parent.width
                text: (modelData.disabled ? "Disabled · " : "") + root.description(modelData)
                color: modelData.disabled ? Theme.surfaceVariantText : Theme.surfaceText
                wrapMode: Text.WrapAnywhere
            }
            Flow {
                width: parent.width
                spacing: Theme.spacingS
                enabled: !root.busy
                DankButton { text: "Edit"; onClicked: root.beginEdit(modelData, null) }
                DankButton { text: modelData.disabled ? "Enable" : "Disable"; onClicked: root.toggleRule(modelData) }
                DankButton { text: "Remove…"; onClicked: { root.editorOpen = false; root.removeIndex = modelData.index } }
            }
            Column {
                width: parent.width
                spacing: Theme.spacingS
                visible: root.removeIndex === modelData.index
                StyledText {
                    width: parent.width
                    text: "Remove this exclusion? Matching applications will be eligible for future saves and restores. Nothing will be launched now."
                    color: Theme.surfaceVariantText
                    wrapMode: Text.WordWrap
                }
                Flow {
                    width: parent.width
                    spacing: Theme.spacingS
                    enabled: !root.busy
                    DankButton { text: "Confirm removal"; onClicked: root.updateRule("remove", modelData.index, null) }
                    DankButton { text: "Cancel"; onClicked: root.removeIndex = -1 }
                }
            }
        }
    }
    StyledText {
        visible: !root.busy && root.rules.length === 0
        text: "No exclusions configured."
        color: Theme.surfaceVariantText
    }
}
