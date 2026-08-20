window.app = window.app || {};
window.app.modals = window.app.modals || {};

window.app.modals.openAuditPreview = function(auditIdOrModel, settings = {
    mode: "write",
    onbeforeopen: null,
    onafteropen: null,
    onbeforeclose: null,
    onafterclose: null,
}) {
    const modal = auditPreviewModal(auditIdOrModel, settings);
    if (!modal) {
        return;
    }

    document.body.appendChild(modal);

    app.modals.open(modal);
};

function stringifyValue(v) {
    if (v === null || typeof v == "undefined") {
        return "null";
    }
    if (typeof v == "object") {
        return JSON.stringify(v);
    }
    return "" + v;
}

function copyJSON(audit) {
    app.utils.copyToClipboard(JSON.stringify(audit, null, 2));
    app.toasts.success("Audit copied to clipboard!");
}

function auditPreviewModal(auditIdOrModel, settings) {
    let modal;

    const mode = settings.mode == "read" ? "read" : "write";

    const data = store({
        isLoading: false,
        audit: null,
        get changesList() {
            const result = [];
            const changes = data.audit?.changes;
            if (changes) {
                for (const key in changes) {
                    const c = changes[key] || {};
                    result.push({ field: key, old: c.old, new: c.new });
                }
            }
            return result;
        },
        get hasSnapshot() {
            return !app.utils.isEmpty(data.audit?.snapshot);
        },
    });

    async function load() {
        data.isLoading = true;

        try {
            if (app.utils.isObject(auditIdOrModel)) {
                data.audit = JSON.parse(JSON.stringify(auditIdOrModel));
            } else {
                // fallback fetch (only the write trail exposes a view endpoint)
                data.audit = await app.pb.send(`/api/audits/${encodeURIComponent(auditIdOrModel)}`, {
                    method: "GET",
                    requestKey: "audit_preview",
                });
            }

            data.isLoading = false;
        } catch (err) {
            if (!err.isAbort) {
                data.isLoading = false;
                app.checkApiError(err);
            }
        }
    }

    function metaRow(label, value) {
        const isEmpty = app.utils.isEmpty(value);
        return t.tr(
            { rid: "audit_meta_" + label },
            t.th({ className: "min-width p-r-0" }, label),
            t.td(
                null,
                isEmpty
                    ? t.span({ className: "txt txt-hint" }, "N/A")
                    : t.span({ className: "txt", textContent: app.utils.displayValue(value, 1000) }),
            ),
            t.td({ className: "col-copy min-width" }, app.components.copyButton(isEmpty ? "" : ("" + value))),
        );
    }

    function metaTable() {
        const fields = mode == "read"
            ? [
                "collection_name",
                "record_id",
                "event",
                "source",
                "auth_collection",
                "auth_id",
                "filter",
                "sort",
                "page",
                "per_page",
                "total_items",
                "user_ip",
                "user_agent",
            ]
            : [
                "collection_name",
                "record_id",
                "event",
                "source",
                "auth_collection",
                "auth_id",
                "user_ip",
                "user_agent",
            ];

        return t.table(
            { className: "audit-view-table responsive-table" },
            t.tbody(
                null,
                t.tr(
                    null,
                    t.th({ className: "min-width p-r-0" }, "id"),
                    t.td(null, () => data.audit.id),
                    t.td({ className: "col-copy min-width" }, app.components.copyButton(data.audit.id)),
                ),
                t.tr(
                    null,
                    t.th({ className: "min-width p-r-0" }, "created"),
                    t.td(
                        null,
                        app.components.formattedDate({
                            value: () => data.audit.created,
                            short: false,
                        }),
                    ),
                    t.td({ className: "col-copy min-width" }, app.components.copyButton(data.audit.created)),
                ),
                fields.map((f) => metaRow(f, data.audit[f])),
            ),
        );
    }

    function changesSection() {
        if (mode != "write" || !data.changesList.length) {
            return;
        }

        return t.div(
            { className: "audit-section" },
            t.div({ className: "section-title" }, "Field changes"),
            t.table(
                { className: "audit-changes-table responsive-table" },
                t.thead(
                    null,
                    t.tr(
                        null,
                        t.th({ className: "min-width" }, "Field"),
                        t.th(null, "Old"),
                        t.th(null, "New"),
                    ),
                ),
                t.tbody(
                    null,
                    data.changesList.map((c) =>
                        t.tr(
                            { rid: "audit_change_" + c.field },
                            t.th({ className: "min-width" }, c.field),
                            t.td(
                                null,
                                t.span({
                                    className: "label sm danger audit-diff-old",
                                    textContent: stringifyValue(c.old),
                                }),
                            ),
                            t.td(
                                null,
                                t.span({
                                    className: "label sm success audit-diff-new",
                                    textContent: stringifyValue(c.new),
                                }),
                            ),
                        )
                    ),
                ),
            ),
        );
    }

    function snapshotSection() {
        if (mode != "write" || !data.hasSnapshot) {
            return;
        }

        return t.div(
            { className: "audit-section" },
            t.div({ className: "section-title" }, "Snapshot"),
            app.components.codeBlock({
                value: JSON.stringify(data.audit.snapshot, null, 2),
            }),
        );
    }

    modal = t.div(
        {
            pbEvent: "auditPreviewModal",
            className: "modal audit-preview-modal",
            onbeforeopen: (el) => {
                load();
                return settings.onbeforeopen?.(el);
            },
            onafteropen: (el) => {
                settings.onafteropen?.(el);
            },
            onbeforeclose: (el) => {
                return settings.onbeforeclose?.(el);
            },
            onafterclose: (el) => {
                settings.onafterclose?.(el);
                el?.remove();
            },
        },
        t.header(
            { className: "modal-header" },
            t.h5(null, mode == "read" ? "Read access details" : "Data change details"),
            t.button(
                {
                    className: "btn sm circle transparent m-l-auto",
                    title: "More options",
                    "html-popovertarget": "audit-meta-dropdown",
                },
                t.i({ className: "ri-more-line", ariaHidden: true }),
            ),
            t.div({ id: "audit-meta-dropdown", className: "dropdown", popover: "auto" }, (el) => {
                return t.button(
                    {
                        className: "dropdown-item",
                        onclick: () => {
                            copyJSON(data.audit);
                            el.hidePopover();
                        },
                    },
                    t.i({ className: "ri-braces-line", ariaHidden: true }),
                    t.span({ className: "txt" }, "Copy JSON"),
                );
            }),
        ),
        t.div({ className: "modal-content" }, () => {
            if (!data.audit || data.isLoading) {
                return t.div({ className: "block txt-center" }, t.span({ className: "loader" }));
            }

            return [
                metaTable(),
                changesSection(),
                snapshotSection(),
            ];
        }),
        t.footer(
            { className: "modal-footer" },
            t.button(
                {
                    type: "button",
                    className: "btn transparent",
                    onclick: () => app.modals.close(modal),
                },
                t.span({ className: "txt" }, "Close"),
            ),
        ),
    );

    return modal;
}
