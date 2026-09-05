import QtQuick
import qs.Common
import qs.Widgets
import qs.Modules.Plugins

PluginSettings {
    id: root
    pluginId: "dankSession"

    StyledText {
        text: "Session restoration"
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Font.Bold
        color: Theme.surfaceText
    }

    ToggleSetting {
        settingKey: "autoRestore"
        label: "Restore after login"
        description: "Restore the most recent session when the DankSession service starts"
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
        label: "Capture interval (seconds)"
        description: "Periodic safety snapshot interval; window events are captured sooner"
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
        text: "Application launch rules live in ~/.config/danksession/config.json. Window titles are excluded by default."
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }
}
