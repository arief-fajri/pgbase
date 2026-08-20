import { auditsList } from "./auditsList";

const TABS = ["write", "read"];

export function pageAudits(route) {
    app.store.title = "Audits";

    const FILTER_QUERY_KEY = "filter";
    const TAB_QUERY_KEY = "tab";

    const queryFilter = route.query[FILTER_QUERY_KEY]?.[0] || "";
    const queryTab = TABS.includes(route.query[TAB_QUERY_KEY]?.[0]) ? route.query[TAB_QUERY_KEY][0] : "write";

    const auditsSettings = store({
        reset: null,
        isListLoading: false,
        hasListItems: false,
        tab: queryTab,
        filter: queryFilter,
        get isLoading() {
            return auditsSettings.isListLoading;
        },
    });

    function refreshList() {
        auditsSettings.reset = Date.now();
    }

    function changeTab(tab) {
        if (auditsSettings.tab == tab) {
            return;
        }
        auditsSettings.tab = tab;
        // clear the search since the two trails don't share all filterable fields
        auditsSettings.filter = "";
    }

    const watchers = [];

    return t.div(
        {
            pbEvent: "pageAudits",
            className: "page page-audits",
            onmount() {
                watchers.push(
                    watch(() => {
                        app.utils.replaceHashQueryParams({
                            [FILTER_QUERY_KEY]: auditsSettings.filter,
                            [TAB_QUERY_KEY]: auditsSettings.tab,
                        });
                    }),
                );
            },
            onunmount() {
                watchers.forEach((w) => w?.unwatch());
            },
        },
        t.div(
            { className: "page-content full-height" },
            t.header(
                { className: "page-header" },
                t.nav({ className: "breadcrumbs" }, t.div({ className: "breadcrumb-item" }, "Audits")),
                t.div(
                    { className: "inline-flex gap-sm" },
                    app.components.refreshButton({
                        onclick: refreshList,
                    }),
                ),
                app.components.searchbar({
                    className: "audits-searchbar",
                    historyKey: "pbAuditsSearchHistory",
                    placeholder: "Search term or filter like `event = 'delete'`",
                    value: () => auditsSettings.filter || "",
                    onsubmit: (val) => auditsSettings.filter = val,
                    autocomplete: [
                        "id",
                        "created",
                        "collection_name",
                        "record_id",
                        "event",
                        "auth_id",
                        "auth_collection",
                        "source",
                        "filter",
                        { value: "changes.", label: "changes.*" },
                    ],
                }),
            ),
            t.div(
                { className: "tabs-header audits-tabs" },
                t.button(
                    {
                        type: "button",
                        className: () => `tab-item ${auditsSettings.tab == "write" ? "active" : ""}`,
                        onclick: () => changeTab("write"),
                    },
                    t.i({ className: "ri-edit-2-line", ariaHidden: true }),
                    t.span({ className: "txt" }, "Data changes"),
                ),
                t.button(
                    {
                        type: "button",
                        className: () => `tab-item ${auditsSettings.tab == "read" ? "active" : ""}`,
                        onclick: () => changeTab("read"),
                    },
                    t.i({ className: "ri-eye-line", ariaHidden: true }),
                    t.span({ className: "txt" }, "Read access"),
                ),
            ),
            auditsList(auditsSettings),
            t.footer({ className: "page-footer" }, app.components.credits()),
        ),
    );
}
