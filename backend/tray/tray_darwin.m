// Prism status bar implementation.
//
// Named with the `prism_tray_` prefix so nothing collides with Wails'
// internal AppDelegate / menu handling. All UI mutations are hopped onto
// the main queue; Wails v2 owns the NSApplication run loop and it is safe
// to post blocks from any thread.

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

extern void prismTrayItemClicked(int id);

@interface PrismTrayDelegate : NSObject
- (void)itemClicked:(id)sender;
@end

@implementation PrismTrayDelegate
- (void)itemClicked:(id)sender {
    NSMenuItem *mi = (NSMenuItem *)sender;
    prismTrayItemClicked((int)[mi tag]);
}
@end

static NSStatusItem       *gStatusItem = nil;
static NSMenu             *gMenu       = nil;
static PrismTrayDelegate  *gDelegate   = nil;
static NSMutableArray     *gItems      = nil;   // NSMenuItem flat index for tag lookup
static int                 gNextTag    = 0;

static void prism_tray_init_main(void) {
    if (gStatusItem != nil) return;

    gItems = [[NSMutableArray alloc] init];
    gDelegate = [[PrismTrayDelegate alloc] init];

    NSStatusBar *bar = [NSStatusBar systemStatusBar];

    // Use a fixed-width status item (thickness of the menu bar). Variable
    // length grows to fit the button's title; we don't want a title so there
    // is no need for it to stretch, and a fixed square keeps the icon as
    // compact as possible — crucial on MacBooks with a notch where every
    // extra point gets pushed off-screen.
    gStatusItem = [[bar statusItemWithLength:[bar thickness]] retain];
    [gStatusItem setVisible:YES];

    if ([gStatusItem respondsToSelector:@selector(button)]) {
        gStatusItem.button.imagePosition = NSImageOnly;
        gStatusItem.button.imageScaling  = NSImageScaleProportionallyDown;
        gStatusItem.button.title         = @"";
    }

    gMenu = [[NSMenu alloc] init];
    [gMenu setAutoenablesItems:NO];
    [gStatusItem setMenu:gMenu];
}

void prism_tray_init(void) {
    if ([NSThread isMainThread]) {
        prism_tray_init_main();
    } else {
        dispatch_sync(dispatch_get_main_queue(), ^{
            prism_tray_init_main();
        });
    }
}

void prism_tray_set_icon(const char *path) {
    if (path == NULL) return;
    NSString *p = [NSString stringWithUTF8String:path];
    if (p == nil) return;

    dispatch_async(dispatch_get_main_queue(), ^{
        if (gStatusItem == nil) return;
        NSImage *img = [[NSImage alloc] initWithContentsOfFile:p];
        if (img == nil) return;

        // Standard menu bar icons are 18pt tall (NSStatusBar thickness is
        // 22, but Apple's HIG reserves 2pt padding top and bottom). Size
        // the image to the icon area rather than the full bar.
        [img setSize:NSMakeSize(18, 18)];

        // Template = YES: macOS recolours the alpha mask to match the
        // current menu bar appearance (monochrome light/dark). This matches
        // every native menu bar app (Wi-Fi, Bluetooth, control center …)
        // and is what users expect. Our prism PNG already has a crisp
        // alpha shape so it templates cleanly.
        [img setTemplate:YES];

        if ([gStatusItem respondsToSelector:@selector(button)]) {
            gStatusItem.button.image         = img;
            gStatusItem.button.imagePosition = NSImageOnly;
            gStatusItem.button.imageScaling  = NSImageScaleProportionallyDown;
        }
        [img release];
    });
}

void prism_tray_set_title(const char *title) {
    if (title == NULL) return;
    NSString *t = [NSString stringWithUTF8String:title];
    if (t == nil) t = @"";

    dispatch_async(dispatch_get_main_queue(), ^{
        if (gStatusItem == nil) return;
        if ([gStatusItem respondsToSelector:@selector(button)]) {
            gStatusItem.button.title = t;
        }
    });
}

int prism_tray_add_item(const char *title, int enabled) {
    if (title == NULL) title = " ";
    NSString *t = [NSString stringWithUTF8String:title];
    if (t == nil) t = @" ";

    __block int assigned = -1;

    dispatch_sync(dispatch_get_main_queue(), ^{
        if (gMenu == nil) return;

        int tag = gNextTag++;
        NSMenuItem *mi = [[NSMenuItem alloc]
            initWithTitle:t
                   action:@selector(itemClicked:)
            keyEquivalent:@""];
        [mi setTarget:gDelegate];
        [mi setTag:tag];
        [mi setEnabled:(enabled ? YES : NO)];

        [gMenu addItem:mi];
        [gItems addObject:mi];
        [mi release];
        assigned = tag;
    });

    return assigned;
}

int prism_tray_add_sub_item(int parentId, const char *title, int enabled) {
    if (title == NULL) title = " ";
    NSString *t = [NSString stringWithUTF8String:title];
    if (t == nil) t = @" ";

    __block int assigned = -1;

    dispatch_sync(dispatch_get_main_queue(), ^{
        NSMenuItem *parent = nil;
        for (NSMenuItem *mi in gItems) {
            if ([mi tag] == parentId) { parent = mi; break; }
        }
        if (parent == nil) return;

        NSMenu *sub = [parent submenu];
        if (sub == nil) {
            sub = [[[NSMenu alloc] init] autorelease];
            [sub setAutoenablesItems:NO];
            [parent setSubmenu:sub];
        }

        int tag = gNextTag++;
        NSMenuItem *mi = [[NSMenuItem alloc]
            initWithTitle:t
                   action:@selector(itemClicked:)
            keyEquivalent:@""];
        [mi setTarget:gDelegate];
        [mi setTag:tag];
        [mi setEnabled:(enabled ? YES : NO)];

        [sub addItem:mi];
        [gItems addObject:mi];
        [mi release];
        assigned = tag;
    });

    return assigned;
}

void prism_tray_add_separator(void) {
    dispatch_sync(dispatch_get_main_queue(), ^{
        if (gMenu == nil) return;
        [gMenu addItem:[NSMenuItem separatorItem]];
    });
}

void prism_tray_update_item_title(int id_, const char *title) {
    if (title == NULL) title = "";
    NSString *t = [NSString stringWithUTF8String:title];
    if (t == nil) t = @"";

    dispatch_async(dispatch_get_main_queue(), ^{
        for (NSMenuItem *mi in gItems) {
            if ([mi tag] == id_) {
                [mi setTitle:t];
                break;
            }
        }
    });
}

void prism_tray_update_item_enabled(int id_, int enabled) {
    dispatch_async(dispatch_get_main_queue(), ^{
        for (NSMenuItem *mi in gItems) {
            if ([mi tag] == id_) {
                [mi setEnabled:(enabled ? YES : NO)];
                break;
            }
        }
    });
}

void prism_tray_set_dock_visible(int visible) {
    // NSApplicationActivationPolicyRegular vs Accessory.
    //
    //   Regular   (0): normal app with Dock icon + menu bar.
    //   Accessory (1): no Dock icon, no app menu bar entry, but NSStatusItem
    //                  still rendered and app keeps running.
    //
    // When the user closes the Wails window via red dot / Cmd+W we switch
    // to Accessory so the process hides like a native status bar app. When
    // the tray is used to reopen the window we switch back to Regular first
    // so Cmd+Tab + Mission Control treat Prism as a full app again.
    NSApplicationActivationPolicy policy =
        visible ? NSApplicationActivationPolicyRegular
                : NSApplicationActivationPolicyAccessory;

    dispatch_async(dispatch_get_main_queue(), ^{
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:policy];
        if (visible) {
            // Bring the app to the foreground so the window actually gets
            // focus after reopening from the menu bar.
            [app activateIgnoringOtherApps:YES];
        }
    });
}
