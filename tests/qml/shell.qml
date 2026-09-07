import QtQuick
import Quickshell

// Layout tests use lightweight visual widgets, not a second DMS instance.
// No backend commands or private files are accessed.
ShellRoot {
    id: test
    // Compile the complete widget as well, but never instantiate its processes.
    Component { SessionWidget {} }
    property int phase: 0
    property bool failed: false
    property real emptyHeight: 0
    property real shortHeight: 0
    property int actions: 0

    function check(value, message) {
        if (value) return
        failed = true
        console.error("FAIL", message)
    }

    function snapshot(count) {
        var rows = []
        for (var i = 0; i < count; i++) {
            rows.push({application: i === 0 ? "Very long application identifier that must not widen the panel" : "browser",
                workspace: {id: i + 1, name: "work"}, width: 1200, height: 800,
                restore: i % 3 === 0 ? "relaunch" : (i % 3 === 1 ? "placement" : "excluded")})
        }
        return {saved: true, daemonRunning: true, savedWindows: rows}
    }

    FloatingWindow {
        visible: true
        implicitWidth: 400
        implicitHeight: 700
        SessionWidget.SessionPopout {
            id: panel
            width: 360
            savedAge: "Saved 1 minute ago"
            onActionRequested: (command, dryRun) => {
                test.check(command === "restore" && dryRun, "preview signal must stay read-only")
                test.actions++
            }
        }
    }

    Timer {
        interval: 100
        running: true
        repeat: true
        onTriggered: {
            switch (test.phase++) {
            case 0:
                test.emptyHeight = panel.implicitHeight
                test.check(test.emptyHeight > 0 && test.emptyHeight < 260, "empty popout should be compact")
                panel.sessionStatus = test.snapshot(2)
                break
            case 1:
                test.shortHeight = panel.implicitHeight
                test.check(test.shortHeight > test.emptyHeight && test.shortHeight < 400, "two rows should fit without fixed blank space")
                test.check(panel.savedWindows.length === 2, "list contains saved rows")
                test.check(panel.restoreLabel("placement") === "Placement only", "unconfigured windows must not imply relaunch")
                panel.actionRequested("restore", true)
                panel.sessionStatus = test.snapshot(30)
                break
            case 2:
                test.check(panel.savedListHeight === 260, "long list must be capped and scrollable")
                test.check(panel.implicitHeight < 500, "long list must not grow the panel indefinitely")
                panel.width = 220
                panel.actionOutput = "An operation failed. ".repeat(100)
                break
            case 3:
                test.check(panel.implicitHeight < 620, "narrow panel and long errors must remain bounded")
                panel.width = 360
                panel.actionOutput = ""
                panel.sessionStatus = test.snapshot(2)
                break
            case 4:
                test.check(Math.abs(panel.implicitHeight - test.shortHeight) < 1, "panel must shrink after list/error changes")
                panel.statusError = true
                break
            case 5:
                test.check(panel.savedWindows.length === 0, "backend failure must not show stale rows")
                test.check(test.actions === 1, "viewing/changing status must not trigger actions")
                console.log(test.failed ? "POPOUT QA FAILED" : "POPOUT QA PASSED")
                Qt.quit()
            }
        }
    }
}
