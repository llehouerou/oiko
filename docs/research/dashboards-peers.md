# Research: how peers model custom dashboards

Ticket: #57, part of map #56 (Custom Dashboards: shared, personal and per Kiosk).

Scope: Home Assistant (HA), openHAB, Homey and Apple Home. For each one, six questions: what a dashboard is made of; whether a grouping can reference a room live or only copy it; whether a card can show part of a device; how layout adapts to a phone; what is per user and what is shared; what breaks or is cleaned up when an entity or room is deleted. Then the pain points users report. Research only: this lists options, trade-offs and evidence, and makes no decision for Oiko.

Freshness: sources were read in early October 2026. HA docs and the `dev` branch of `home-assistant/frontend` and `home-assistant/core` were current with HA 2026.9. openHAB docs were Stable 5.2.1 / Latest 5.3.0, and openHAB source was the `main` branches. Homey and Apple support articles carry their own "updated" dates (2026).

Labels: **[direct]** means the cited source says it. **[interp]** is my reading of a source. **[inference]** is my own reasoning and no source states it. Peer vocabulary is kept inside each product's section. The "Relevance to Oiko" section uses Oiko's terms (Dashboard, Tile, Area, Layout, Kiosk, Person).

---

## Key findings

1. **The shared structure is dashboard → page or view → group → card, stored as one document per dashboard.**
   - HA: dashboard → views (tabs) → sections → cards.
   - openHAB: a page is a tree of components; a layout page holds blocks → rows/columns or cells → widgets.
   - Homey: a dashboard is `{name, columns[]}` of widgets.
   - Each saves the whole dashboard or page as one JSON document: HA `.storage/lovelace.<id>`, openHAB JSON DB `uicomponents_ui_page`, Homey `PUT /api/manager/dashboards/dashboard/:id`.
2. **Live reference versus copy is the central trade-off, and HA ships both.**
   - Live: generated dashboards and views (strategies) are rebuilt from the registries of entities, devices and areas. The `area` view strategy and the area card take an area id and show whatever is in the area now.
   - Copy: "Take control" turns a generated dashboard into a static copy, and it is irreversible. openHAB's "Add from Model" and HA's "Add to dashboard" from a device page also copy.
   - Both openHAB and HA document that a copy goes stale. openHAB issue #893 says it makes the feature "pretty useless".
   - HA has a middle ground: a live area view with per-dashboard `hidden` and `order` overrides, keyed by entity ID.
3. **Cards are per entity or per Item, so "part of a device" is the default in HA and openHAB.**
   - HA cards point at entities. A device has many entities, including configuration and diagnostic ones.
   - openHAB widgets point at Items: one Point = one channel of an Equipment.
   - Apple Home is the opposite: one tile per accessory, even for a fan with a light. The tile's appearance can be changed per accessory.
   - Homey widgets take a device, or a list of devices filtered by class and capability.
4. **Phone layout: the robust designs keep a small fixed grid inside a group and let groups reflow.**
   - HA's sections view gives each section a 12-column grid. Sections flow left to right ("Z-Grid") into as many 320–500 px columns as fit, so a phone gets one column of sections in order. HA chose this after users found the height-driven Masonry unpredictable across screens.
   - openHAB needs per-column breakpoints set by hand, or a fixed grid sized for one screen.
   - Homey shows one column per swipe on a phone and several on a tablet.
   - Users in every tool still ask for a different arrangement on the phone. Workarounds: duplicate cards with screen conditions (HA), `visible: =device.desktop` (openHAB), or separate pages per device.
5. **None of the four lets an ordinary user own a personal dashboard on the hub.**
   - HA and openHAB dashboards and pages are shared, and only admins may save them. This is enforced server-side: `@websocket_api.require_admin` in HA, `@RolesAllowed({ Role.ADMIN })` in openHAB.
   - "Per user" in both means hiding things per user: HA `visible` on views and a `user` condition on cards and sections; openHAB `visibleTo`. openHAB's docs warn: "Do not be fooled into assuming the 'visible only to' feature gives you any security". HA's say a hidden view's "URL path is still accessible".
   - What is per user in HA: the default dashboard, which was a system-wide default plus a per-user override in 2025.12 and per device before that, and the sidebar order (stored in the user profile since 2025.6).
   - Homey's personal surface is a "Home Screen" that is "private and saved on-device". Its Dashboards live on the Homey.
6. **Wall tablets are handled with a dedicated user per screen or a client-side kiosk lock.**
   - HA's docs say to give a wall tablet its own user. Users protested when 2025.12 dropped the per-device default dashboard.
   - Homey's Kiosk Mode locks the app to one dashboard behind the owner's account password. Users reported that a Guest account could not start it.
   - openHAB offers a `hideLeftPanel=true` URL parameter and fixed-size layouts.
7. **Nothing is cleaned up when something a dashboard references is deleted.** References are by mutable name, so renaming breaks them too.
   - HA cards point at `entity_id` strings. Renaming or deleting an entity leaves "Entity not found" warnings. HA's own dialogs say: "This will not change any configuration (like automations, scripts, scenes, dashboards) … You will have to manually edit them yourself". A deleted area shows "Area not found".
   - openHAB Items are keyed by name and cannot be renamed. Every widget and page that references one is "to be checked and edited" by hand.
   - Apple Home: removing a room moves its accessories to the Default Room.
   - The generated views (HA strategies, openHAB model tabs, Apple's room list) are the only ones that never go stale.
8. **Concurrent editing is detected, not merged.** HA saves the whole dashboard document. A second editor sees "Dashboard updated in another session. Refreshing will discard your unsaved changes."

---

## Home Assistant

### What a dashboard is made of
- **[direct]** "Each dashboard is made up of cards." Dashboards hold views. "A view is a tab inside a dashboard" and "Views control the layout". There are four view types: Sections (the default), Masonry, Panel and Sidebar. [Views](https://www.home-assistant.io/dashboards/views/), [Cards](https://www.home-assistant.io/dashboards/cards/)
- **[direct]** A sections view holds sections, and sections hold cards. "A heading card will be automatically added to the top of the section." Sections can have a background, a theme and visibility conditions. A view can also have a header (title and badges) and a sticky footer card. [Sections view](https://www.home-assistant.io/dashboards/sections/)
- **[direct]** "Subviews won't show up in the navigation bar"; cards reach them through a navigate action. [Views](https://www.home-assistant.io/dashboards/views/)
- **[direct]** Built-in dashboards ("Home", "Lights", "Security", "Climate", "Maintenance", "Energy", "Map"…) "cannot be deleted, and there are limited options on how much you can edit them". Users can create any number of other dashboards, each optionally in the sidebar. [Dashboards](https://www.home-assistant.io/dashboards/dashboards/)
- **[direct]** Storage: each UI dashboard is one stored document (`CONFIG_STORAGE_KEY = "lovelace.{}"`). The list of dashboards is a separate collection (`lovelace_dashboards`) with `title`, `icon`, `show_in_sidebar` and `require_admin`. `async_save` replaces the whole `config`. [core lovelace/dashboard.py](https://github.com/home-assistant/core/blob/dev/homeassistant/components/lovelace/dashboard.py), [core lovelace/const.py](https://github.com/home-assistant/core/blob/dev/homeassistant/components/lovelace/const.py)

### Live reference to an area, or a copy
- **[direct]** Strategies "generate custom dashboards and/or views". "Built in dashboards are built with dashboard strategies." A view strategy can be placed as one view of an otherwise hand-made dashboard (`views: - strategy: type: …`). [Custom strategies (developer docs)](https://developers.home-assistant.io/docs/frontend/custom-ui/custom-strategy/)
- **[direct]** Built-in view strategies include `area` and `home-area`. The `area` view strategy takes `area` (an area id) and `groups_options` with per-group `hidden` and `order` lists of entities. It throws "Unknown area" when the area is gone. A `LovelaceStrategy` declares `registryDependencies` (entities, devices, areas, floors…) and may define `shouldRegenerate`. Section strategies also exist; the only built-in one is `common-controls`. [get-strategy.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/strategies/get-strategy.ts), [area-view-strategy.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/strategies/areas/area-view-strategy.ts), [strategies/types.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/strategies/types.ts)
- **[interp]** Generated content therefore follows changes in those registries: a device moved to another area shows up in the new area's view without anyone editing a dashboard.
- **[direct]** The Home dashboard "shows your entities grouped by areas … the dashboard provides a separate view for each area". Its source builds one `home-area` view per area and takes a `favorite_entities` list. [Dashboards](https://www.home-assistant.io/dashboards/dashboards/), [home-dashboard-strategy.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/strategies/home/home-dashboard-strategy.ts)
- **[direct]** Taking control of a generated dashboard means "it is no longer automatically updated when new dashboard elements become available. Once you've taken control, you can't get this specific dashboard back to update automatically." [Dashboards](https://www.home-assistant.io/dashboards/dashboards/)
- **[direct]** The area card takes `area` (an id) and shows live what is assigned to that area. Examples: the median temperature and humidity, a motion alert, a camera feed, and "Area controls" buttons. `exclude_entities` removes members. To control only some devices of a room as a unit, the docs suggest creating another area. [Area card](https://www.home-assistant.io/dashboards/area/)
- **[direct]** "Add to dashboard" on a device page adds that device's entities as tile cards "inside a new section". [Dashboard chapter 1 blog](https://www.home-assistant.io/blog/2024/03/04/dashboard-chapter-1/), [Cards](https://www.home-assistant.io/dashboards/cards/)
- **[interp]** That device-page addition is a copy taken when it is added. The result is a list of entity IDs in the section, with no link back to the device.
- **[direct]** Since 2025.12, a manual order of floors and areas (Settings > Areas) applies "instantly … to all built-in dashboards that show areas and floors". It answered a pain point, the "strict ordering of areas (alphabetically)". [2025.12 release notes](https://www.home-assistant.io/blog/2025/12/03/release-202512/)

### Part of a device
- **[direct]** "An entity represents a sensor, actor, or function", and it "is usually part of a device". Cards are added "By entity", and a tile card shows one entity, with optional "features" (quick controls) that depend on "the card and entity capabilities". [Dashboards glossary tooltip](https://www.home-assistant.io/dashboards/dashboards/), [Cards](https://www.home-assistant.io/dashboards/cards/), [Tile card](https://www.home-assistant.io/dashboards/tile/)
- **[interp]** A card can therefore show any single function of a device, down to one configuration or diagnostic entity. There is no device card that groups a device's entities by meaning.

### Layout and phones
- **[direct]** Users found Masonry unpredictable: "Even a difference in height of 1 pixel would mean a card … getting shifted all the way to the right", and "the muscle memory of where users remember the cards will be lost every time the dashboard is displayed on another screen". HA concluded that "the Masonry layout, compatibility with multiple screen sizes, and easy 'drag and drop' rearrangement of cards cannot co-exist". [Dashboard chapter 1 blog](https://www.home-assistant.io/blog/2024/03/04/dashboard-chapter-1/)
- **[direct]** In the sections view, "The view will rearrange the sections according to the amount of space available horizontally, while the number of columns of cards within each section stays the same". Sections are laid out by the "Z-Grid", left to right, starting a new row when the row is full. A row is as tall as its tallest section. [Dashboard chapter 1 blog](https://www.home-assistant.io/blog/2024/03/04/dashboard-chapter-1/)
- **[direct]** Source: the number of section columns is `floor(width / (min column width + gap))`. The minimum is `--column-min-width: 320px` and the maximum `500px`. That count is clamped to `max_columns` (default 4). A section's `column_span` is clamped to the available columns, and `dense_section_placement` lets the grid fill gaps. [hui-sections-view.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/views/hui-sections-view.ts), [views/const.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/views/const.ts)
- **[direct]** "Each section is divided in 12 columns". A cell is about 30 px wide and 56 px high with an 8 px gap. A card declares default, min and max `rows` and `columns`. Without them it takes 12 columns and ignores rows. [Custom card (developer docs)](https://developers.home-assistant.io/docs/frontend/custom-ui/custom-card/)
- **[direct]** Inside a section, cards get `grid-column: span …` and `grid-row: span …` with no start cell. [hui-grid-section.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/sections/hui-grid-section.ts)
- **[interp]** A card's place is its order plus its size: HA stores no explicit cell, and an empty cell cannot be kept on purpose.
- **[direct]** Cards and sections can be shown by screen size with `condition: screen` and any CSS media query. [Conditional card](https://www.home-assistant.io/dashboards/conditional/)

### Per user versus shared
- **[direct]** Saving and deleting a dashboard's config are admin-only: `@websocket_api.require_admin` on `lovelace/config/save` and `lovelace/config/delete`. [core lovelace/websocket.py](https://github.com/home-assistant/core/blob/dev/homeassistant/components/lovelace/websocket.py)
- **[direct]** A dashboard can be "visible only to the admin user" (`require_admin`). A view's `visible` lists user ids. The docs note: "This is only for the display of the tabs. The URL path is still accessible". Cards, badges and sections have a `user` visibility condition, and with no conditions they are shown "to all users". [Dashboards](https://www.home-assistant.io/dashboards/dashboards/), [Views](https://www.home-assistant.io/dashboards/views/), [Cards](https://www.home-assistant.io/dashboards/cards/), [Sections view](https://www.home-assistant.io/dashboards/sections/)
- **[direct]** Default dashboard: an admin sets one "for all users", and any user may override it in their profile. "If you set your phone to one dashboard and your wall tablet to another, using the same user, they'll both revert to the default dashboard"; "If you want your wall tablet to use a different dashboard … use a separate user profile". [Dashboards](https://www.home-assistant.io/dashboards/dashboards/), [2025.12 release notes](https://www.home-assistant.io/blog/2025/12/03/release-202512/)
- **[direct]** Since 2025.6, "The sidebar customization is now stored in your user profile". Before that it was per device and was sometimes lost. [2025.6 release notes](https://www.home-assistant.io/blog/2025/06/11/release-20256/)

### Deletion and renaming
- **[direct]** A tile whose entity is missing renders a warning: "Entity not found". An area card whose area is missing shows "Area not found". [hui-tile-card.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/cards/hui-tile-card.ts), [hui-warning.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/components/hui-warning.ts), [hui-area-card.ts](https://github.com/home-assistant/frontend/blob/dev/src/panels/lovelace/cards/hui-area-card.ts)
- **[direct]** HA's own dialog texts:
  - Renaming entity IDs: "This will not change any configuration (like automations, scripts, scenes, dashboards) that is currently using these entities! You will have to manually edit them yourself".
  - Deleting entities: "Remove them from your dashboard and automations if they include these entities."
  - Deleting an entity that is related to other things: "If you delete it, you will need to update those manually."

  [frontend en.json](https://github.com/home-assistant/frontend/blob/dev/src/translations/en.json)
- **[direct]** "Entity IDs are used to reference entities in automations, scripts, and dashboards." [frontend en.json](https://github.com/home-assistant/frontend/blob/dev/src/translations/en.json)

### Pain points reported
- **Per-user dashboards.** These are long-standing requests:
  - [WTH can't I set a default dashboard per user (2022)](https://community.home-assistant.io/t/wth-cant-i-set-a-default-dashboard-per-user/467847)
  - [WTH why can't we have easily per user dashboards (2022)](https://community.home-assistant.io/t/wth-why-cant-we-have-easily-per-user-dashboards/472886)
  - [WTH assign dashboard to person or user (2024)](https://community.home-assistant.io/t/wth-assign-dashboard-to-person-or-user/803089)

  The usual workaround is a third-party add-on that hides the sidebar and forces a dashboard per user: [How to: Per user Dashboard](https://community.home-assistant.io/t/how-to-per-user-dashboard/656305).
- **Per device versus per user.** After 2025.12 removed the per-device default, users of one account on a PC and a phone objected: "Removing this seems to defeat the purpose of creating multiple dashboards!" They also objected that "it would be absurd to have a separate user for every single device". [Default dashboard per user/device? (2026)](https://community.home-assistant.io/t/default-dashboard-per-user-device/988518)
- **Phone layout.** On a phone, a sections dashboard "collapses to 1 section", and a card sized for a two-section-wide layout stays small. The workaround reported: "Use the same card twice with different column settings and a visibility settings with different media screen size queries." [Section Based Dashboard: Enlarge grid_columns on mobile? (2025)](https://community.home-assistant.io/t/section-based-dashboard-enlarge-grid-columns-on-mobile/950390). See also [Sections on mobile device: too small (2024)](https://community.home-assistant.io/t/sections-on-mobile-device-too-small/783099).
- **Stale references.** Seasonal devices disabled for part of the year leave "entities not found errors" on dashboards. The suggested workaround is a visibility condition. [Hide entities on the dashboard when disabled / not found (2025)](https://community.home-assistant.io/t/hide-entities-on-the-dashboard-when-disabled-not-found/842973). A user also saw "Entity not found" on a generated dashboard, which can only be fixed by taking control. [(2025)](https://community.home-assistant.io/t/entity-not-found-in-default-overview-dashboard/951513)
- **Concurrent edits.** These are detected only at the document level: "Dashboard updated in another session. Refreshing will discard your unsaved changes." [frontend en.json](https://github.com/home-assistant/frontend/blob/dev/src/translations/en.json)

---

## openHAB (Main UI)

### What a dashboard is made of
- **[direct]** Pages are trees of UI components. Each component has a `type`, `config` and named `slots` holding child components. A page is a root component with a `uid`, `props` and `tags`. [Building Pages](https://www.openhab.org/docs/ui/building-pages.html)
- **[direct]** The page types are Layout (Responsive or Fixed), Map, Floor plan, Chart and Tabbed. "Tabbed Pages are Pages used to combine several other Pages and render them in tabs". Older sitemaps are a separate model that the Main UI does not render. [User Interface Design Overview](https://www.openhab.org/docs/ui/)
- **[direct]** "A Responsive Layout page can host one or multiple blocks, optionally followed by a masonry". A block contains "rows or cells containers", and a row contains columns that each host one widget. [Responsive Layout Pages](https://www.openhab.org/docs/ui/layout-pages-responsive.html)
- **[direct]** Pages are stored by `ManagedUIComponentProvider` in a storage named `"uicomponents_" + namespace`, one entry per root component (page). [ManagedUIComponentProvider.java](https://github.com/openhab/openhab-core/blob/main/bundles/org.openhab.core.ui/src/main/java/org/openhab/core/ui/internal/components/ManagedUIComponentProvider.java)

### Live reference to a location, or a copy
- **[direct]** The home page has an Overview tab "you can customize entirely from scratch", plus three tabs that are "an automatically generated view of your model" (Locations, Equipment, Properties). Their cards "appear automatically as you build your model". [User Interface Design Overview](https://www.openhab.org/docs/ui/)
- **[direct]** On the generated tabs, an admin can reorder cards, insert named separators (for example by floor), hide cards, and override a card's title and background. [Overview Page tutorial](https://www.openhab.org/docs/tutorial/auto_overview.html)
- **[direct]** "Add from Model..." places each chosen Item's default widget, but "The widget that will be put on the page is a copy … if you change this definition for an item, widgets that were already put on a page … will NOT be updated." [Responsive Layout Pages](https://www.openhab.org/docs/ui/layout-pages-responsive.html)
- **[direct]** The issue on that point: "no widget that has been added to a page gets updated if the item metadata gets changed later on … (one time copy)". The maintainer classed it as an "expected & documented shortcoming" and an enhancement. [openhab-webui #893](https://github.com/openhab/openhab-webui/issues/893)
- **[direct]** Live membership is possible with `oh-repeater` (`sourceType: itemsInGroup` or `itemsWithTags`) and with `oh-location-card`, "A card showing model items in a certain location", which takes a location `item`. [oh-repeater](https://github.com/openhab/openhab-webui/blob/main/bundles/org.openhab.ui/doc/components/oh-repeater.md), [oh-location-card](https://github.com/openhab/openhab-webui/blob/main/bundles/org.openhab.ui/doc/components/oh-location-card.md)

### Part of a device
- **[direct]** In the semantic model, a Location is a Group, an Equipment "is normally a Group Item", and "A Point is not a Group, but represents any other type of Item and is usually linked to a Channel". [Semantic Model tutorial](https://www.openhab.org/docs/tutorial/model.html)
- **[interp]** A standalone widget is bound to one Item, that is one Point, so a widget shows part of a device by default. The Equipment card on the generated tabs lists every Point under the equipment's name.

### Layout and phones
- **[direct]** "Responsive Layouts are the main layouts in openHAB and recommended for most uses". "Fixed Grid Layouts are more suitable for wall-mounted tablets or other devices with a fixed screen size". The layout type "cannot be changed afterwards". [Layout Pages](https://www.openhab.org/docs/ui/layout-pages.html)
- **[direct]** In a responsive layout, cells and masonry adapt by themselves. Rows and columns need per-column widths at breakpoints (`width`, `xsmall` ≥480 px … `xlarge` ≥1200 px). The docs advise building "for the narrow screens first". [Responsive Layout Pages](https://www.openhab.org/docs/ui/layout-pages-responsive.html)
- **[direct]** A Fixed Grid starts from a screen size (default 1280×720) and a column count (default 16). Optional scaling "can lead to unpredictable styling issues". [Fixed Layout Pages](https://www.openhab.org/docs/ui/layout-pages-fixed.html)

### Per user versus shared
- **[direct]** Writing pages is admin-only: `@RolesAllowed({ Role.ADMIN })` on POST, PUT and DELETE of `/ui/components/{namespace}`. Reading is allowed for USER and ADMIN. [UIResource.java](https://github.com/openhab/openhab-core/blob/main/bundles/org.openhab.core.io.rest.ui/src/main/java/org/openhab/core/io/rest/ui/internal/UIResource.java)
- **[direct]** `visibleTo` on a page or widget takes `role:…` or `user:<userid>`. The docs warn: "Do not be fooled into assuming the 'visible only to' feature gives you any security". A page appears in the sidebar if its own "Show on Sidebar" is on, at the place set by its own integer "Sidebar order". For kiosks, `hideLeftPanel=true` in the URL hides the sidebar. [User Interface Design Overview](https://www.openhab.org/docs/ui/), [Building Pages](https://www.openhab.org/docs/ui/building-pages.html)
- **[direct]** "Regular users … can see all of the interactive parts of the UI (i.e. Pages)". "every administrator … will need to create a custom interface for the users". [Pages intro tutorial](https://www.openhab.org/docs/tutorial/pages_intro.html)

### Deletion and renaming
- **[direct]** The Item name is "the unique key for each Item … technically you cannot rename an Item, you have to destroy the old one and make a new one". Afterwards, cross-references in "MainUI widgets and pages, all to be checked and edited" by hand. [OH3: rename items in UI (community)](https://community.openhab.org/t/oh3-rename-items-in-ui/128575)
- **[interp]** Nothing removes a deleted Item from pages: widgets reference Items by name inside the page document. I did not find what a widget renders for a missing Item. *Not verified.*

### Pain points reported
- **Hiding per user leaves holes.** "the cell gets rendered empty and uses up the space" when `visibleTo` hides a widget inside masonry or cells. [How to hide oh-cell when using visibleTo (2023)](https://community.openhab.org/t/how-to-hide-oh-cell-when-using-visibleto-in-oh-masonry-or-oh-grid-cells/143885)
- **No finer per-user control than hiding.** Users ask to restrict kids or guests to some controls. The answer: "OH does not have fine grained controls like that … Access to all Items is allowed for all types of users." [Restricting Access to OH3 Overview and Pages (2022)](https://community.openhab.org/t/restricting-access-to-oh3-overview-and-pages-to-selected-users-other-than-visibleto/134874)
- **A separate phone layout.** A 2–3 column overview "doesn't work that nicely" on phones. The suggested fix is `visible: =device.desktop` on every component not wanted on a phone. [MainUI layout for an Android phone (2022)](https://community.openhab.org/t/mainui-layout-for-an-android-phone/140897)
- **Copies go stale.** See [#893](https://github.com/openhab/openhab-webui/issues/893) above.

---

## Homey

### What a dashboard is made of
- **[direct]** "you can create personalized dashboards with widgets … multiple dashboards for different use cases, such as First Floor, Home Security, or Game Room". Widgets come from the "Homey tab" (built-in) or the "Apps tab" (community apps). A dashboard can be edited, renamed, duplicated or deleted. [Create and manage Homey Dashboards](https://support.homey.app/hc/en-us/articles/16732145289116-Create-and-manage-Homey-Dashboards)
- **[direct]** In the API, a `Dashboard` has `id`, `name` and `columns` (array). It is created, updated and deleted through `/api/manager/dashboards/dashboard`, with scopes `homey.dashboard` and `homey.dashboard.readonly`. [ManagerDashboards](https://athombv.github.io/node-homey-api/HomeyAPIV3Local.ManagerDashboards.html), [Dashboard](https://athombv.github.io/node-homey-api/HomeyAPIV3Local.ManagerDashboards.Dashboard.html)
- **[direct]** App widgets are web pages (HTML/CSS/JS) with settings. "Each time a user adds a widget to a dashboard a unique id is generated". [Widgets (Apps SDK)](https://apps.developer.homey.app/the-basics/widgets)
- **[direct]** Dashboards are in the mobile app only: "Homey Dashboards and Homey Energy are only available in the Homey Mobile App." [Use the Homey Web App](https://support.homey.app/hc/en-us/articles/29572162626204-Use-the-Homey-Web-App)

### Live reference to a zone, or a copy
- **[direct]** A zone has a `parent` (zones nest). [Zone](https://athombv.github.io/node-homey-api/HomeyAPIV3Local.ManagerZones.Zone.html)
- **[direct]** "Hidden Devices" hides devices per zone from the Devices overview. [Hide devices](https://support.homey.app/hc/en-us/articles/26624855218844-Hide-devices-in-Homey)
- **[direct]** A widget's `devices` setting selects one or several devices, filtered by `class` and `capabilities`, and users can reorder them. [Widget settings (Apps SDK)](https://apps.developer.homey.app/the-basics/widgets/settings)
- **[interp]** Widgets select devices, not zones. I found no first-party widget that shows a zone live. *Not verified:* the list of built-in widgets is not documented on the pages I read.

### Part of a device
- **[interp]** A widget is bound to devices, and its own code decides which capabilities it shows. The capability filter at selection time restricts which devices are offered, not what is shown. [Widget settings (Apps SDK)](https://apps.developer.homey.app/the-basics/widgets/settings)

### Layout and phones
- **[direct]** On phones, "you can also swipe right to add a column … your dashboards can accommodate up to fifteen columns". On tablets, "your dashboards will display multiple columns at once". Widgets are moved by long-press and drag. [Create and manage Homey Dashboards](https://support.homey.app/hc/en-us/articles/16732145289116-Create-and-manage-Homey-Dashboards)
- **[direct]** A widget's height is fixed, given as a percentage (an aspect ratio) or set at runtime, and then cached to avoid layout shifts. [Widgets (Apps SDK)](https://apps.developer.homey.app/the-basics/widgets)
- **[interp]** The layout is columns of stacked widgets, with no grid inside a column. A phone shows one column per screen.

### Per user versus shared
- **[direct]** The Home Screen is "Private and saved on-device": "This private dashboard is accessible only to you and saved on your device." [Customize your Home Screen](https://support.homey.app/hc/en-us/articles/16502176331164-Customize-your-Home-Screen)
- **[inference]** Dashboards are stored on the Homey with no owner field in the API model, so they are shared by everyone who has `homey.dashboard.readonly`.
- **[direct]** The roles are Owner, Manager, Resident and Guest. Guests are "Control-only" on devices and zones. [Add and manage user accounts](https://support.homey.app/hc/en-us/articles/360012615814-Add-and-manage-user-accounts-on-Homey)
- **[direct]** Kiosk Mode limits "the app to a single Dashboard". Starting and exiting it requires "your Homey account password". For full lockdown, users are sent to OS features (Guided Access, App Pinning). [Start a Homey Dashboard in Kiosk Mode](https://support.homey.app/hc/en-us/articles/20283185947164-Start-a-Homey-Dashboard-in-Kiosk-Mode)

### Deletion
- I found no first-party statement on what a widget shows when its device or zone is deleted. *Not verified.*

### Pain points reported
- **Guests and kiosks.** A guest account could not start Kiosk Mode, and promoting the account to a full user let guests "make changes to the configurations of the items on the dashboard". The poster reported that support promised a fix. Another user summarised it as "Homescreen = user bound, Dashboard = device bound", and said "the dashboard misses out on some crucial user management & locking options". [Dashboard in kiosk mode for a guest user (2025)](https://community.homey.app/t/dashboard-in-kiosk-mode-for-a-guest-user/138421)
- **Restricting a user to one dashboard.** Requested: "my son only have access to a specific dashboard". Staff answered that this is what Kiosk Mode is. [User Control dashboard view (2024)](https://community.homey.app/t/featurerequest-user-control-dashboard-view/123634)

---

## Apple Home

### What a dashboard is made of
- **[direct]** There is one Home tab with sections: "Categories" (Climate, Lights, Security, Speakers & TVs, Water), "Cameras", "Scenes", "Favorites" and "Rooms". [Intro to Home on iPhone](https://support.apple.com/guide/iphone/intro-to-home-iph22d98bbca/ios)
- **[direct]** Users can "Rearrange items" (Edit Home View, then drag tiles), "Reorder sections", and "Resize icons" per tile. [Intro to Home on iPhone](https://support.apple.com/guide/iphone/intro-to-home-iph22d98bbca/ios), [Arrange rooms (Mac)](https://support.apple.com/guide/home/arrange-rooms-hmea9c3e49ed/mac)
- **[interp]** There are no user-created dashboards. Customisation is limited to which accessories are Favorites, the order of tiles and sections, and tile size.

### Rooms versus custom groupings
- **[direct]** The groupings are:
  - **Rooms**, each with its own Room View.
  - **Zones**, which "group rooms … so that accessories in multiple rooms are controlled at once" (with Siri, for example). Zones cannot be renamed.
  - **Accessory groups** ("Group with Other Accessories"), controlled as one tile and optionally a Favorite.
  - **Categories**, generated and "organized by room".

  [Set up accessories (iPhone)](https://support.apple.com/guide/iphone/set-up-accessories-iph125110541/ios), [Group rooms into a zone (Mac)](https://support.apple.com/guide/home/group-rooms-hme6660dd21e/mac), [Control accessories (iPhone)](https://support.apple.com/guide/iphone/control-accessories-iph0a717a8fd/ios), [Organize accessories](https://support.apple.com/en-us/126180)
- **[interp]** Every grouping is live: rooms, zones and categories are views over assignments, never copies. A custom grouping that is not a room exists only as an accessory group, which is a control unit and not a screen section.

### Part of a device
- **[direct]** "Accessories with multiple features are controlled by one accessory tile", for example a fan with a light. "Control-click an accessory tile to change how the tile appears in Home." [Control Home accessories (Mac)](https://support.apple.com/guide/home/overview-hmeb738f98cf/mac)
- **[interp]** The default is one tile per accessory, unlike HA and openHAB. Splitting a multi-feature accessory into several tiles is a per-accessory display option, but its exact wording is not in the guides I read.

### Layout and phones
- **[interp]** The Home tab is one ordered list of sections with tiles in a fixed-width grid. The same arrangement serves iPhone, iPad and Mac, and Apple's guides document no per-screen layout. [Intro to Home on iPhone](https://support.apple.com/guide/iphone/intro-to-home-iph22d98bbca/ios)

### Per user versus shared
- **[direct]** Residents "have local and optional remote access to all accessories". Guests "have local-only access to specific doors, locks, and security system accessories on a set schedule". [Invite others (iPhone)](https://support.apple.com/guide/iphone/invite-others-to-control-accessories-iphcbaf7e8f3/ios)
- **Not verified:** whether Favorites and the Home View arrangement are per person or shared by the home. Apple's guides I read do not say.

### Deletion
- **[direct]** "When you remove the room, the accessories assigned to it move to Default Room." [Set up accessories (iPhone)](https://support.apple.com/guide/iphone/set-up-accessories-iph125110541/ios)

---

## Comparison

| | Home Assistant | openHAB | Homey | Apple Home |
|---|---|---|---|---|
| Made of | dashboards → views → sections → cards | pages (component tree): blocks → rows/cols or cells → widgets; tabbed pages | dashboards → columns → widgets; private Home Screen | one Home tab: sections (Favorites, Scenes, Rooms…) of tiles |
| Room reused live | yes: strategies (Home, `area` view with hidden/order), area card | yes: generated Locations tab, `oh-location-card`, `oh-repeater` over a group | not found | yes: Room View, categories by room; zones |
| Copy | "Take control" (irreversible), "Add to dashboard" | "Add from Model" (documented as stale) | — | — |
| Unit of a card | one entity (can be one setting) | one Item (one Point) | devices chosen per widget | one accessory (split optional) |
| Phone | sections reflow to one column; 12-col grid kept inside each section; screen conditions | per-column breakpoints by hand; fixed grid per screen | one column per swipe, several on tablet | same list |
| Per user | default dashboard and sidebar order; views and cards hidden per user (cosmetic) | `visibleTo` (cosmetic) | Home Screen private on-device | not documented |
| Who edits | admins only (server-enforced) | admins only (server-enforced) | holders of scope `homey.dashboard` (which roles: not documented) | not documented |
| Wall tablet | a dedicated user | `hideLeftPanel`, fixed layout | Kiosk Mode (owner password) | — |
| Deleted/renamed reference | "Entity not found" / "Area not found"; no clean-up; rename breaks | by name; no rename; manual clean-up | not found | room removed → Default Room |

---

## Relevance to Oiko (options, not decisions)

These notes connect the evidence to the open questions of map #56. Every item is **[inference]**.

- **Reusing a whole Area.** Peers offer it live (HA's `area` view strategy and area card, openHAB's Locations tab) or as a copy (HA "Take control", openHAB "Add from Model"). Both peers document that copies go stale, and users complain about it.
  - A live reference fits the map's "whole Areas reused with everything they hold". It is closer to how Oiko's built-in Dashboard already works.
  - HA's `groups_options.hidden/order` shows a cheap middle ground: a live Area plus per-Dashboard exclusions and order.
  - Open question for Oiko: whether such overrides are worth having, and how they relate to the Area's own Layout (ADR 0015).
- **References by identity avoid the peers' worst breakage.** HA and openHAB reference by mutable name, so a rename breaks dashboards. Oiko already refers to Targets and Areas by identity (`CONTEXT.md`: Target, Name).
  - What remains is deletion. Peers either leave a visible "not found" placeholder (HA) or rehome the content (Apple's Default Room).
  - Oiko's Update stream already announces "a Target deleted", so dropping the reference or showing a placeholder are both open.
- **Unit of a Tile.** HA and openHAB cards show one entity or Item, which makes them flexible but turns every device into many cards. Apple and Homey stay at the accessory or device level.
  - ADR 0014 places Oiko with Apple: one Tile per Target, shaped by Roles.
  - The peers show that "part of a device" is mostly asked for multi-function hardware: Apple's per-accessory "change how the tile appears" for a fan with a light. In Oiko, a Function per quantity already covers part of this.
- **Layout.** HA abandoned height-driven Masonry for a grid with explicit sizes inside groups. This is the same reasoning as ADR 0015's rejection of an auto-filled grid.
  - HA stores order plus span with no explicit cell, while Oiko's Layout stores a cell and keeps empty cells on purpose.
  - On a phone, HA keeps each section's internal grid because a section is never wider than about 500 px. Oiko's Area Layout of up to 6 columns becomes an order instead.
  - Users of every peer still ask for a different phone arrangement and work around it with duplicated, screen-conditional cards. That is a signal to watch, not a reason to store two Layouts (rejected in ADR 0015).
- **Ownership.** No peer lets a non-admin own a dashboard on the hub. HA and openHAB enforce admin-only writes, and Homey's only private surface lives on the device.
  - Oiko's personal Dashboards for every Person, Guests included, go beyond the peers. The peers' complaints show the demand: per-user dashboards, a default per user, kids and guests confined to one dashboard.
  - openHAB warns that per-user visibility is no security, and HA that a hidden view stays reachable by URL. That matches Oiko's settled rule that a Dashboard only arranges what its viewer may already see.
- **Kiosks.** HA's advice is a dedicated user per wall tablet, and its 2025.12 change caused protests from single-user households. Homey locks a dashboard on a device behind the owner's password.
  - Oiko's Kiosk is already an identity of its own (ADR 0022, ADR 0029), and the map says an Admin assigns it a Dashboard.
  - The peers suggest this is the right grain: per screen, not per user or browser.
- **Storage and concurrent edits.** All peers store one document per dashboard and save it whole, which fits ADR 0003 and ADR 0032. HA only detects a concurrent save and offers a refresh that discards changes. This is relevant to the map's "live updates reach a Dashboard being edited elsewhere".

## Not verified / gaps

- What an openHAB widget renders for a deleted Item.
- Whether Homey has a built-in widget that shows a whole zone live.
- What a Homey widget shows when its device is deleted.
- Whether Apple Home's Favorites and Home View arrangement are per person or shared, and the exact name of the "show as separate tiles" option. Apple's guides read for this note do not state them, and no first-party source was found.
- Forum threads are first-party community spaces, but individual posts are user reports and not product statements. They are cited as pain points only.
