import { defineConfig } from "vitepress";
import { withMermaid } from "vitepress-plugin-mermaid";

export default withMermaid(defineConfig({
    // ELK layout engine (registered in theme/index.js) — orthogonal edge routing
    // + crossing minimisation for the dense architecture flowchart.
    mermaid: {
        layout: "elk",
        flowchart: { nodeSpacing: 40, rankSpacing: 60 },
    },
    title: "PGBase Docs",
    description: "PostgreSQL-powered backend as a service (PocketBase fork) — architecture, flows, features.",
    // Project pages are served from a subpath: https://arief-fajri.github.io/pgbase/
    // Local dev therefore serves at http://localhost:5174/pgbase/
    base: "/pgbase/",
    srcDir: ".",
    outDir: ".vitepress/dist",
    cacheDir: ".vitepress/cache",
    lastUpdated: true,
    head: [
        // Base-aware: files in public/ are served from the configured base ("/pgbase/").
        ["link", { rel: "icon", type: "image/svg+xml", href: "/pgbase/favicon.svg" }],
        ["meta", { name: "theme-color", content: "#336791" }],
    ],
    themeConfig: {
        logo: "/logo.svg",
        outline: [2, 3],
        // Audience-based navigation: three tracks surfaced as dropdowns.
        nav: [
            {
                text: "Build",
                items: [
                    { text: "Fork deltas", link: "/fork-deltas" },
                    { text: "Collections & API rules", link: "/collections-and-api-rules" },
                    { text: "Auth flows", link: "/flows/auth" },
                    { text: "Realtime flows", link: "/flows/realtime" },
                ],
            },
            {
                text: "Operate",
                items: [
                    { text: "Single-host production", link: "/single-host-production" },
                    { text: "Production runbook", link: "/production" },
                    { text: "Backup, restore & observability", link: "/backup-restore-observability" },
                ],
            },
            {
                text: "Contribute",
                items: [
                    { text: "Contributing", link: "/contributing" },
                    { text: "Developing", link: "/developing" },
                    { text: "Releasing", link: "/contributing-releasing" },
                    { text: "Architecture", link: "/architecture/end-to-end" },
                ],
            },
            {
                text: "Reference",
                items: [
                    { text: "Env variables", link: "/reference/env" },
                    { text: "Roadmap", link: "/roadmap" },
                ],
            },
        ],
        // Single audience-grouped sidebar, shown site-wide.
        sidebar: [
            {
                text: "Get started",
                items: [
                    { text: "Overview", link: "/" },
                    { text: "For AI agents", link: "/agents" },
                    { text: "Comparison & positioning", link: "/comparison" },
                    { text: "Fork deltas", link: "/fork-deltas" },
                ],
            },
            {
                text: "Build",
                collapsed: false,
                items: [
                    { text: "Collections & API rules", link: "/collections-and-api-rules" },
                    { text: "Auth flows", link: "/flows/auth" },
                    { text: "Realtime flows", link: "/flows/realtime" },
                ],
            },
            {
                text: "Operate",
                collapsed: false,
                items: [
                    { text: "Single-host production", link: "/single-host-production" },
                    { text: "Production runbook", link: "/production" },
                    { text: "Backup, restore & observability", link: "/backup-restore-observability" },
                    { text: "Env reference", link: "/reference/env" },
                ],
            },
            {
                text: "Contribute",
                collapsed: false,
                items: [
                    { text: "Contributing", link: "/contributing" },
                    { text: "Developing", link: "/developing" },
                    { text: "Releasing", link: "/contributing-releasing" },
                    {
                        text: "Architecture",
                        collapsed: true,
                        items: [
                            { text: "End-to-end", link: "/architecture/end-to-end" },
                            { text: "Backend layers", link: "/architecture/backend-layers" },
                            { text: "Audit design", link: "/architecture/audit-design" },
                        ],
                    },
                ],
            },
            {
                text: "Project",
                items: [{ text: "Roadmap", link: "/roadmap" }],
            },
        ],
        search: { provider: "local" },
        editLink: {
            pattern: "https://github.com/arief-fajri/pgbase/edit/main/docs/:path",
        },
        socialLinks: [{ icon: "github", link: "https://github.com/arief-fajri/pgbase" }],
        footer: {
            message: "Fork of PocketBase · MIT Licensed",
            copyright: "© 2026 PGBase",
        },
    },
    markdown: {
        config(md) {
            const defaultFence = md.renderer.rules.fence.bind(md.renderer.rules);
            md.renderer.rules.fence = (tokens, idx, options, env, slf) => {
                const token = tokens[idx];
                const info = token.info.trim();
                if (info === "mermaid") {
                    try {
                        return [
                            "<MermaidViewer",
                            ` id="mermaid-${idx}"`,
                            ` graph="${encodeURIComponent(token.content)}"`,
                            "></MermaidViewer>",
                        ].join("");
                    } catch (e) {
                        return `<pre>${e}</pre>`;
                    }
                }
                return defaultFence(tokens, idx, options, env, slf);
            };
        },
    },
    vite: {
        server: { port: 5174 },
        preview: { port: 5174 },
        optimizeDeps: {
            include: [
                "mermaid",
                "@mermaid-js/layout-elk",
                "fastdom",
                "fastdom/extensions/fastdom-promised.js",
                "d3-selection",
                "d3-zoom",
            ],
        },
    },
}));
