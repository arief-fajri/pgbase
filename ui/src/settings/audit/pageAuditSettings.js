import { settingsSidebar } from "../settingsSidebar";

export function pageAuditSettings() {
    app.store.title = "Audit logs";

    const uniqueId = "audit_" + app.utils.randomString();

    const data = store({
        isLoading: false,
        isSaving: false,
        collections: [],
        filter: "",
        formSettings: null,
        originalFormSettings: null,
        get originalFormSettingsHash() {
            return JSON.stringify(data.originalFormSettings);
        },
        get formSettingsHash() {
            return JSON.stringify(data.formSettings);
        },
        get hasChanges() {
            return data.originalFormSettingsHash != data.formSettingsHash;
        },
        get selectedCollections() {
            return app.utils.toArray(data.formSettings?.collections);
        },
        get totalSelected() {
            return data.selectedCollections.length;
        },
        get areAllSelected() {
            return data.collections.length && data.collections.length == data.totalSelected;
        },
    });

    load();

    async function load() {
        data.isLoading = true;

        try {
            const [settings, collections] = await Promise.all([
                app.pb.settings.getAll({ requestKey: uniqueId + "_settings" }),
                app.pb.collections.getFullList({ requestKey: uniqueId + "_collections" }),
            ]);

            // only non-system, non-view collections can be audited (matches the backend allowlist)
            data.collections = collections
                .filter((c) => !c.system && c.type != "view")
                .sort((a, b) => a.name.localeCompare(b.name));

            init(settings);

            data.isLoading = false;
        } catch (err) {
            if (!err.isAbort) {
                app.checkApiError(err);
                // data.isLoading = false; don't reset in case of a server error
            }
        }
    }

    function init(settings = {}) {
        app.store.settings = JSON.parse(JSON.stringify(settings));

        const audit = settings.audit || {};

        data.originalFormSettings = {
            enabled: !!audit.enabled,
            collections: app.utils.toArray(audit.collections),
            retentionDays: audit.retentionDays || 0,
            logIP: !!audit.logIP,
            readEnabled: !!audit.readEnabled,
            readRetentionDays: audit.readRetentionDays || 0,
        };

        data.formSettings = JSON.parse(JSON.stringify(data.originalFormSettings));
    }

    function reset() {
        data.formSettings = JSON.parse(data.originalFormSettingsHash);
    }

    async function save() {
        if (data.isSaving || !data.hasChanges) {
            return;
        }

        data.isSaving = true;

        try {
            const redacted = app.utils.filterRedactedProps({ audit: data.formSettings });

            const updated = await app.pb.settings.update(redacted);

            init(updated);

            app.toasts.success("Successfully saved audit settings.");
        } catch (err) {
            app.checkApiError(err);
        }

        data.isSaving = false;
    }

    function isCollectionSelected(name) {
        return data.selectedCollections.includes(name);
    }

    function toggleCollection(name, state) {
        const set = new Set(data.formSettings.collections);
        if (state) {
            set.add(name);
        } else {
            set.delete(name);
        }
        data.formSettings.collections = Array.from(set);
    }

    function toggleSelectAll() {
        if (data.areAllSelected) {
            data.formSettings.collections = [];
        } else {
            data.formSettings.collections = data.collections.map((c) => c.name);
        }
    }

    function switchField(key, label) {
        const id = uniqueId + "_" + key;
        return t.div(
            { className: "field" },
            t.input({
                id: id,
                name: "audit." + key,
                type: "checkbox",
                className: "switch",
                checked: () => !!data.formSettings[key],
                onchange: (e) => (data.formSettings[key] = e.target.checked),
            }),
            t.label(
                { htmlFor: id },
                t.span({ className: "txt" }, label),
            ),
        );
    }

    function retentionField(key, enabledKey) {
        const id = uniqueId + "_" + key;
        return t.div(
            { className: "field" },
            t.label({ htmlFor: id }, t.span({ className: "txt" }, "Retention (days)")),
            t.input({
                id: id,
                name: "audit." + key,
                type: "number",
                min: 0,
                placeholder: "0 = keep forever",
                disabled: () => !data.formSettings[enabledKey],
                value: () => data.formSettings[key] || "",
                oninput: (e) => (data.formSettings[key] = e.target.value << 0),
            }),
        );
    }

    function trailCard(enabledKey, title, description, retentionKey) {
        return t.div(
            { className: "audit-card" },
            t.div(
                { className: "audit-card-head" },
                switchField(enabledKey, title),
            ),
            t.div(
                { className: "grid audit-card-body" },
                t.div(
                    { className: "col-sm-5 col-lg-4" },
                    retentionField(retentionKey, enabledKey),
                ),
                t.div(
                    { className: "col-sm-7 col-lg-8 audit-card-info" },
                    t.div({ className: "txt-sm txt-hint" }, description),
                    t.div({ className: "field-help" }, "0 = keep forever"),
                ),
            ),
        );
    }

    function allowlistWarning() {
        const trailOn = data.formSettings.enabled || data.formSettings.readEnabled;
        if (!trailOn || data.totalSelected > 0) {
            return undefined;
        }
        return t.div(
            { className: "alert warning m-b-sm" },
            t.i({ className: "ri-error-warning-line" }),
            t.div(
                { className: "content" },
                "One or more trails are enabled but no collections are selected — nothing will be recorded. Select at least one collection below.",
            ),
        );
    }

    function collectionsList() {
        return t.div(
            { className: "audit-collections" },
            t.div(
                { className: "audit-collections-header" },
                t.div(
                    { className: "audit-collections-heading" },
                    t.div(
                        { className: "audit-collections-title" },
                        t.span({ className: "txt" }, "Audited collections"),
                        () =>
                            t.span(
                                { className: "audit-collections-count" },
                                "(" + data.totalSelected + " / " + data.collections.length + ")",
                            ),
                    ),
                    t.div(
                        { className: "audit-collections-subtitle txt-sm txt-hint" },
                        "The collection allowlist below applies to both trails.",
                    ),
                ),
                t.div({ className: "flex-fill" }),
                t.button(
                    {
                        type: "button",
                        className: "btn sm secondary transparent",
                        disabled: () => !data.collections.length,
                        onclick: () => toggleSelectAll(),
                    },
                    () => t.span({ className: "txt" }, data.areAllSelected ? "Clear all" : "Select all"),
                ),
            ),
            () => {
                if (!data.collections.length) {
                    return t.div({ className: "txt-hint" }, "No auditable collections found.");
                }

                return t.div(
                    { className: "audit-collections-body" },
                    t.div(
                        { className: "audit-collections-search fields searchbar" },
                        t.div(
                            { className: "field addon p-r-0" },
                            t.i({ className: "ri-search-line txt-hint", ariaHidden: true }),
                        ),
                        t.div(
                            { className: "field" },
                            t.input({
                                className: "p-l-5",
                                type: "text",
                                placeholder: "Search collections...",
                                value: () => data.filter,
                                oninput: (e) => (data.filter = e.target.value),
                            }),
                        ),
                        () => {
                            if (!data.filter.length) {
                                return undefined;
                            }
                            return t.div(
                                { className: "field addon p-l-0 p-r-5 gap-0" },
                                t.button(
                                    {
                                        type: "button",
                                        className: "btn sm circle transparent secondary",
                                        ariaDescription: app.attrs.tooltip("Clear", "left"),
                                        onclick: () => (data.filter = ""),
                                    },
                                    t.i({ className: "ri-close-line", ariaHidden: true }),
                                ),
                            );
                        },
                    ),
                    t.div(
                        { className: "audit-collections-box" },
                        () => {
                            const term = data.filter.trim().toLowerCase();
                            const filtered = term
                                ? data.collections.filter((c) => c.name.toLowerCase().includes(term))
                                : data.collections;

                            if (!filtered.length) {
                                return t.div(
                                    { className: "audit-collections-empty txt-hint" },
                                    "No collections match \"" + data.filter + "\".",
                                );
                            }

                            return t.div(
                                { className: "audit-collections-grid" },
                                filtered.map((collection) => {
                                    const cbId = uniqueId + "_c_" + collection.id;
                                    return t.div(
                                        { className: "field audit-collection-item" },
                                        t.input({
                                            id: cbId,
                                            type: "checkbox",
                                            className: "no-error",
                                            checked: () => isCollectionSelected(collection.name),
                                            onchange: (e) => toggleCollection(collection.name, e.target.checked),
                                        }),
                                        t.label(
                                            { htmlFor: cbId },
                                            t.span({ className: "txt" }, collection.name),
                                            collection.type == "auth"
                                                ? t.span({ className: "label sm" }, "auth")
                                                : undefined,
                                        ),
                                    );
                                }),
                            );
                        },
                    ),
                );
            },
        );
    }

    return t.div(
        {
            pbEvent: "pageAuditSettings",
            className: "page page-audit-settings",
        },
        settingsSidebar(),
        t.div(
            { className: "page-content full-height" },
            t.header(
                { className: "page-header" },
                t.nav(
                    { className: "breadcrumbs" },
                    t.div({ className: "breadcrumb-item" }, "Settings"),
                    t.div({ className: "breadcrumb-item" }, () => app.store.title),
                ),
            ),
            t.div({ className: "wrapper m-b-base" }, () => {
                if (data.isLoading || !data.formSettings) {
                    return t.div({ className: "block txt-center" }, t.span({ className: "loader lg" }));
                }

                return t.form(
                    {
                        pbEvent: "auditSettingsForm",
                        className: "audit-settings-form",
                        inert: () => data.isSaving,
                        onsubmit: (e) => {
                            e.preventDefault();
                            save();
                        },
                    },
                    trailCard(
                        "enabled",
                        "Data changes trail",
                        "Records the create/update/delete operations (with field diffs and snapshots) for the selected collections.",
                        "retentionDays",
                    ),
                    trailCard(
                        "readEnabled",
                        "Read access trail",
                        "Records the view/list access (metadata only — filter/sort/page, never record content) for the selected collections. Best-effort and non-blocking, but can be high-volume on busy collections.",
                        "readRetentionDays",
                    ),
                    t.div(
                        { className: "audit-card" },
                        switchField("logIP", "Store actor IP and User-Agent"),
                        t.div(
                            { className: "field-help" },
                            "When enabled, the request IP and User-Agent of the acting user are stored in both trails.",
                        ),
                        t.hr({ className: "audit-card-divider" }),
                        () => allowlistWarning(),
                        collectionsList(),
                    ),
                    t.hr({ style: "margin:0" }),
                    t.div(
                        { className: "flex audit-actions" },
                        t.div({ className: "m-r-auto" }),
                        t.button(
                            {
                                type: "button",
                                className: "btn transparent secondary",
                                disabled: () => data.isSaving,
                                hidden: () => !data.hasChanges,
                                onclick: reset,
                            },
                            t.span({ className: "txt" }, "Cancel"),
                        ),
                        t.button(
                            {
                                className: () => `btn expanded-lg ${data.isSaving ? "loading" : ""}`,
                                disabled: () => !data.hasChanges || data.isSaving,
                            },
                            t.span({ className: "txt" }, "Save changes"),
                        ),
                    ),
                );
            }),
            t.footer({ className: "page-footer" }, app.components.credits()),
        ),
    );
}
