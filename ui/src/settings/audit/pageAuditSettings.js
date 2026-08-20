import { settingsSidebar } from "../settingsSidebar";

export function pageAuditSettings() {
    app.store.title = "Audit logs";

    const uniqueId = "audit_" + app.utils.randomString();

    const data = store({
        isLoading: false,
        isSaving: false,
        collections: [],
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

    function switchField(key, label, description) {
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
                description
                    ? t.i({
                        className: "ri-information-line link-hint",
                        ariaDescription: app.attrs.tooltip(description),
                    })
                    : undefined,
            ),
        );
    }

    function retentionField(key, label) {
        const id = uniqueId + "_" + key;
        return t.div(
            { className: "field" },
            t.label({ htmlFor: id }, t.span({ className: "txt" }, label)),
            t.input({
                id: id,
                name: "audit." + key,
                type: "number",
                min: 0,
                placeholder: "Keep forever",
                value: () => data.formSettings[key] || "",
                oninput: (e) => (data.formSettings[key] = e.target.value << 0),
            }),
            t.div({ className: "help-block" }, t.small({ className: "txt-hint" }, "Days to keep. 0 = keep forever.")),
        );
    }

    function collectionsList() {
        return t.div(
            { className: "field" },
            t.label(null, t.span({ className: "txt" }, "Audited collections")),
            () => {
                if (!data.collections.length) {
                    return t.div({ className: "txt-hint" }, "No auditable collections found.");
                }

                return t.div(
                    { className: "audit-collections-list" },
                    t.div(
                        { className: "list-item" },
                        t.div(
                            { className: "field" },
                            t.input({
                                id: uniqueId + "_select_all",
                                type: "checkbox",
                                className: "no-error",
                                checked: () => data.areAllSelected,
                                onchange: () => toggleSelectAll(),
                            }),
                            t.label({ htmlFor: uniqueId + "_select_all" }, t.strong(null, "Select all")),
                        ),
                    ),
                    data.collections.map((collection) => {
                        const cbId = uniqueId + "_c_" + collection.id;
                        return t.div(
                            { className: "list-item" },
                            t.div(
                                { className: "field" },
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
                            ),
                        );
                    }),
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
                        className: "grid",
                        inert: () => data.isSaving,
                        onsubmit: (e) => {
                            e.preventDefault();
                            save();
                        },
                    },
                    t.div(
                        { className: "col-lg-12" },
                        t.div({ className: "section-title" }, "Data changes trail"),
                        t.div(
                            { className: "txt-sm txt-hint m-b-sm" },
                            "Records the create/update/delete operations (with field diffs and snapshots) for the selected collections.",
                        ),
                    ),
                    t.div({ className: "col-lg-8" }, switchField("enabled", "Enable data changes trail")),
                    t.div({ className: "col-lg-4" }, retentionField("retentionDays", "Retention (days)")),
                    t.div(
                        { className: "col-lg-12" },
                        switchField(
                            "logIP",
                            "Store actor IP and User-Agent",
                            "When enabled, the request IP and User-Agent of the acting user are stored in the trail.",
                        ),
                    ),
                    t.div({ className: "col-lg-12" }, t.hr()),
                    t.div(
                        { className: "col-lg-12" },
                        t.div({ className: "section-title" }, "Read access trail"),
                        t.div(
                            { className: "txt-sm txt-hint m-b-sm" },
                            "Records the view/list access (metadata only — filter/sort/page, never record content) for the selected collections. Best-effort and non-blocking, but can be high-volume on busy collections.",
                        ),
                    ),
                    t.div({ className: "col-lg-8" }, switchField("readEnabled", "Enable read access trail")),
                    t.div({ className: "col-lg-4" }, retentionField("readRetentionDays", "Retention (days)")),
                    t.div({ className: "col-lg-12" }, t.hr()),
                    t.div(
                        { className: "col-lg-12" },
                        t.div(
                            { className: "txt-sm txt-hint m-b-sm" },
                            "The collection allowlist below applies to both trails.",
                        ),
                        collectionsList(),
                    ),
                    t.div({ className: "col-lg-12" }, t.hr()),
                    t.div(
                        { className: "col-lg-12" },
                        t.div(
                            { className: "flex" },
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
                    ),
                );
            }),
            t.footer({ className: "page-footer" }, app.components.credits()),
        ),
    );
}
