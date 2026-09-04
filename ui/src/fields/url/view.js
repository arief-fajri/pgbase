// {
//     record: undefined,
//     field: undefined,
//     short: false,
// }
export function view(props) {
    return t.div(
        { className: "record-field-view field-type-url" },
        () => {
            const value = props.record[props.field.name] || "";

            if (!value) {
                return t.span({ className: "missing-value" });
            }

            const className = `txt ${props.short ? "txt-ellipsis" : ""}`;

            // PGB-L07: render a clickable link only for allow-listed URL
            // schemes / relative paths. Anything else (javascript:, data:,
            // vbscript:, etc.) is rendered as inert text so a stored value that
            // bypassed server-side validation can never execute in the
            // privileged dashboard origin.
            if (!isSafeUrl(value)) {
                return t.span({
                    className,
                    textContent: app.utils.truncate(value),
                });
            }

            return t.a({
                href: () => value,
                className,
                rel: "noopener noreferrer",
                target: "_blank",
                textContent: app.utils.truncate(value),
                ariaDescription: app.attrs.tooltip("Open in new tab"),
                onclick: (e) => {
                    e.stopPropagation();
                },
            });
        },
    );
}

// isSafeUrl returns true for relative/plain URLs and the allow-listed URL
// schemes, false for dangerous or exotic schemes (PGB-L07).
function isSafeUrl(value) {
    if (value.startsWith("/") || value.startsWith("./") || value.startsWith("../")) {
        return true;
    }

    const match = value.match(/^([a-zA-Z][a-zA-Z0-9+.-]*):/);
    if (!match) {
        // no scheme -> treated as a safe relative/plain URL
        return true;
    }

    switch (match[1].toLowerCase()) {
        case "http":
        case "https":
        case "mailto":
        case "tel":
        case "ftp":
            return true;
        default:
            return false;
    }
}
