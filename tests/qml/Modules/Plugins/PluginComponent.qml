import QtQuick
Item {
    property string pluginId: ""
    property var pluginService: null
    property Component horizontalBarPill: null
    property Component verticalBarPill: null
    property Component popoutContent: null
    property int iconSize: 16
    property real popoutWidth: 400
    property real popoutHeight: 0
    function closePopout() {}
}
