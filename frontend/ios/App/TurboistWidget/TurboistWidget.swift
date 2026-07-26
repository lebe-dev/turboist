import WidgetKit
import SwiftUI

// Lock-screen accessory widget: a single round "+" button. It carries no live
// data — tapping it opens the containing app via the `turboist://quick-add` URL,
// which the SPA turns into the QuickAdd dialog (see frontend/src/lib/native/deepLink.ts).

private let quickAddURL = URL(string: "turboist://quick-add")

struct QuickAddEntry: TimelineEntry {
    let date: Date
}

struct QuickAddProvider: TimelineProvider {
    func placeholder(in context: Context) -> QuickAddEntry {
        QuickAddEntry(date: Date())
    }

    func getSnapshot(in context: Context, completion: @escaping (QuickAddEntry) -> Void) {
        completion(QuickAddEntry(date: Date()))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<QuickAddEntry>) -> Void) {
        // Static launcher: one entry that never needs refreshing.
        completion(Timeline(entries: [QuickAddEntry(date: Date())], policy: .never))
    }
}

struct QuickAddWidgetView: View {
    var entry: QuickAddProvider.Entry

    var body: some View {
        ZStack {
            AccessoryWidgetBackground()
            Image(systemName: "plus")
                .font(.system(size: 22, weight: .semibold))
        }
        .widgetURL(quickAddURL)
        .modifier(ClearContainerBackground())
    }
}

// `containerBackground` is required on iOS 17+ but unavailable on 16, where the
// lock-screen accessory family already exists — apply it only where it compiles.
private struct ClearContainerBackground: ViewModifier {
    func body(content: Content) -> some View {
        if #available(iOS 17.0, *) {
            content.containerBackground(for: .widget) { Color.clear }
        } else {
            content
        }
    }
}

struct QuickAddWidget: Widget {
    let kind = "TurboistQuickAddWidget"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: QuickAddProvider()) { entry in
            QuickAddWidgetView(entry: entry)
        }
        .configurationDisplayName("New task")
        .description("Open Turboist and start a new task.")
        .supportedFamilies([.accessoryCircular])
    }
}

@main
struct TurboistWidgetBundle: WidgetBundle {
    var body: some Widget {
        QuickAddWidget()
    }
}
