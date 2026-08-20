const perPage = 50;

export function auditsList(auditsSettings) {
    const data = store({
        audits: [],
        lastLoadCount: 0,
        lastPage: 1,
        get canLoadMore() {
            return data.lastLoadCount >= perPage;
        },
    });

    // used as a loose guard to prevent freshly inserted rows from constantly
    // pushing the older ones to later pages while paginating
    let loadStartDate;

    async function load(reset = false) {
        auditsSettings.isListLoading = true;

        try {
            let page;
            if (reset) {
                page = 1;
                loadStartDate = app.utils.toRFC3339Datetime(new Date());
            } else {
                page = data.lastPage + 1;
            }

            const isRead = auditsSettings.tab == "read";
            const path = isRead ? "/api/audits/reads" : "/api/audits";
            const searchFields = isRead
                ? ["collection_name", "record_id", "event", "auth_id", "filter"]
                : ["collection_name", "record_id", "event", "auth_id"];

            const filters = [
                app.utils.normalizeSearchFilter(auditsSettings.filter, searchFields),
                `created <= "${loadStartDate}"`,
            ];

            const result = await app.pb.send(path, {
                method: "GET",
                requestKey: "audits_list",
                query: {
                    page: page,
                    perPage: perPage,
                    skipTotal: 1,
                    sort: "-created",
                    filter: filters
                        .filter(Boolean)
                        .map((f) => "(" + f + ")")
                        .join("&&"),
                },
            });

            if (result.page == 1) {
                data.audits = [];
            }

            data.lastPage = result.page;
            data.lastLoadCount = result.items.length;

            for (let i = 0; i < result.items.length; i++) {
                app.utils.pushOrReplaceObject(data.audits, result.items[i]);

                // yield to main
                if (i > 1 && i % 20 == 0) {
                    await new Promise((r) => setTimeout(r, 0));
                }
            }

            auditsSettings.isListLoading = false;
            auditsSettings.hasListItems = data.audits.length > 0;
        } catch (err) {
            if (!err.isAbort) {
                auditsSettings.isListLoading = false;
                app.checkApiError(err);
            }
        }
    }

    function openPreview(audit) {
        app.modals.openAuditPreview(audit, { mode: auditsSettings.tab });
    }

    function colHeader(icon, label) {
        return t.div(
            { className: "inline-flex gap-5" },
            t.i({ className: icon, ariaHidden: true }),
            t.span({ textContent: label }),
        );
    }

    function eventLabel(event) {
        let cls = "";
        if (event == "create") {
            cls = "success";
        } else if (event == "update") {
            cls = "warning";
        } else if (event == "delete") {
            cls = "danger";
        }
        return t.span({ className: `label sm audit-event-label ${cls}` }, event);
    }

    function actorContent(audit) {
        if (!audit.auth_id) {
            return t.span({ className: "txt-hint" }, audit.source || "system");
        }
        return t.div(
            { className: "inline-flex gap-5 flex-wrap" },
            t.span({ className: "label sm" }, audit.auth_collection || "auth"),
            t.span({ className: "txt" }, app.utils.truncate(audit.auth_id, 50)),
        );
    }

    function targetContent(audit) {
        if (auditsSettings.tab == "read" && audit.event == "list") {
            if (!audit.filter) {
                return t.span({ className: "txt-hint" }, "—");
            }
            return t.span({ className: "label sm audit-filter-label" }, app.utils.truncate(audit.filter, 120));
        }
        if (!audit.record_id) {
            return t.span({ className: "txt-hint" }, "—");
        }
        return t.span({ className: "txt txt-mono" }, audit.record_id);
    }

    function auditRow(audit) {
        return t.tr(
            {
                rid: audit.id,
                tabIndex: 0,
                role: "button",
                className: "handle",
                onclick: () => openPreview(audit),
                onkeypress: (e) => {
                    if (e.key == "Enter" || e.key == " ") {
                        e.preventDefault();
                        openPreview(audit);
                    }
                },
            },
            t.td(
                { className: "col-field-name-created" },
                app.components.formattedDate({ value: () => audit.created, short: false }),
            ),
            t.td({ className: "col-field-name-collection" }, t.span({ className: "txt" }, () => audit.collection_name)),
            t.td({ className: "col-field-name-event" }, () => eventLabel(audit.event)),
            t.td({ className: "col-field-name-auth" }, () => actorContent(audit)),
            t.td({ className: "col-field-name-target" }, () => targetContent(audit)),
            t.td({ className: "col-meta" }, t.i({ className: "ri-arrow-right-line", ariaHidden: true })),
        );
    }

    function emptyRow() {
        return t.tr(
            null,
            t.td(
                { colSpan: 99 },
                () => {
                    if (auditsSettings.isListLoading) {
                        return t.span({ className: "skeleton-loader" });
                    }

                    return t.div(
                        { className: "sticky-content txt-center txt-hint" },
                        t.div({ className: "txt-bold" }, "No audit logs found."),
                        t.button(
                            {
                                hidden: () => !auditsSettings.filter?.length,
                                type: "button",
                                className: "btn secondary expanded-lg m-t-10",
                                onclick() {
                                    auditsSettings.filter = "";
                                },
                            },
                            t.span({ className: "txt" }, "Clear search"),
                        ),
                    );
                },
            ),
        );
    }

    function loadMoreRow() {
        return t.tr(
            { hidden: () => !data.canLoadMore },
            t.td(
                { colSpan: 99 },
                t.button(
                    {
                        className: () =>
                            `btn lg secondary load-more-btn ${
                                auditsSettings.isListLoading ? "transparent loading" : ""
                            }`,
                        disabled: () => auditsSettings.isListLoading,
                        onclick: () => load(),
                    },
                    t.span({ className: "txt" }, "Load older"),
                ),
            ),
        );
    }

    const watchers = [];

    return t.div(
        {
            pbEvent: "auditsList",
            className: "page-table-wrapper",
            onmount(el) {
                watchers.push(
                    watch(
                        () => [auditsSettings.reset, auditsSettings.tab, auditsSettings.filter],
                        () => {
                            load(true);

                            if (el) {
                                el.scrollTop = 0;
                            }
                        },
                    ),
                );
            },
            onunmount() {
                watchers.forEach((w) => w?.unwatch());
            },
        },
        t.table(
            { className: "audits-table" },
            t.thead(
                null,
                t.tr(
                    null,
                    t.th({ className: "col-field-name-created" }, colHeader("ri-calendar-line", "Created")),
                    t.th({ className: "col-field-name-collection" }, colHeader("ri-folder-2-line", "Collection")),
                    t.th({ className: "col-field-name-event" }, colHeader("ri-flashlight-line", "Event")),
                    t.th({ className: "col-field-name-auth" }, colHeader("ri-user-line", "Auth")),
                    t.th(
                        { className: "col-field-name-target" },
                        () =>
                            auditsSettings.tab == "read"
                                ? colHeader("ri-filter-3-line", "Record / Filter")
                                : colHeader("ri-fingerprint-line", "Record"),
                    ),
                    t.th({ className: "col-meta" }),
                ),
            ),
            t.tbody(
                null,
                () => {
                    if (!data.audits?.length) {
                        return emptyRow();
                    }

                    return data.audits.map((audit) => auditRow(audit));
                },
                loadMoreRow(),
            ),
        ),
    );
}
