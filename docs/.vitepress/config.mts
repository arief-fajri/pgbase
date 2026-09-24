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
    description: "The simple, self-hosted application backend for PostgreSQL. One binary. Your database.",
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
        // Navigation: 5 items with dropdowns for Build, Deploy, Reference.
        nav: [
            { text: "Getting Started", link: "/getting-started" },
            {
                text: "Build",
                items: [
                    { text: "Collections & API Rules", link: "/collections-and-api-rules" },
                    { text: "Auth Flows", link: "/flows/auth" },
                    { text: "Realtime Flows", link: "/flows/realtime" },
                ],
            },
            {
                text: "Deploy",
                items: [
                    { text: "Single-Host Setup", link: "/deployment/single-host" },
                    { text: "Production Runbook", link: "/deployment/production" },
                    { text: "Disaster Recovery", link: "/deployment/disaster-recovery" },
                ],
            },
            {
                text: "Reference",
                items: [
                    { text: "Environment Variables", link: "/reference/env" },
                    { text: "API Overview", link: "/reference/api-overview" },
                    { text: "API Contract", link: "/reference/api-contract" },
                ],
            },
            { text: "Development", link: "/contributor/" },
        ],
        // Sidebar: 6 sections.
        sidebar: [
            {
                text: "Get Started",
                items: [
                    { text: "Overview", link: "/" },
                    { text: "Getting Started", link: "/getting-started" },
                    { text: "Comparison", link: "/comparison" },
                    { text: "Migrate from PocketBase", link: "/migrate" },
                ],
            },
            {
                text: "Build",
                items: [
                    { text: "Collections & API Rules", link: "/collections-and-api-rules" },
                    { text: "Authentication Flows", link: "/flows/auth" },
                    { text: "Realtime Flows", link: "/flows/realtime" },
                ],
            },
            {
                text: "Deploy",
                items: [
                    { text: "Single-Host Setup", link: "/deployment/single-host" },
                    { text: "Production Runbook", link: "/deployment/production" },
                    { text: "Disaster Recovery", link: "/deployment/disaster-recovery" },
                ],
            },
            {
                text: "Architecture",
                items: [
                    { text: "System Overview", link: "/architecture/overview" },
                    { text: "Backend Layers", link: "/architecture/backend-layers" },
                    { text: "Audit Trail", link: "/architecture/audit-design" },
                ],
            },
            {
                text: "Reference",
                items: [
                    { text: "Environment Variables", link: "/reference/env" },
                    { text: "API Overview", link: "/reference/api-overview" },
                    { text: "API Contract", link: "/reference/api-contract" },
                ],
            },
            {
                text: "Development",
                collapsed: false,
                items: [
                    { text: "Overview", link: "/contributor/" },
                    { text: "Development Setup", link: "/contributor/developing" },
                    { text: "Contributing Guide", link: "/contributor/contributing" },
                    { text: "Release Process", link: "/contributor/releasing" },
                    {
                        text: "Engineering Methodology",
                        collapsed: true,
                        items: [
                            { text: "Platform Design", link: "/contributor/methodology/platform-design" },
                            { text: "Quality Guardrails", link: "/contributor/methodology/guardrails" },
                            { text: "Failure Analysis", link: "/contributor/methodology/failure-modes" },
                            { text: "Observability", link: "/contributor/methodology/observability" },
                            { text: "Evaluation Checklists", link: "/contributor/methodology/checklists" },
                        ],
                    },
                    { text: "Roadmap", link: "/contributor/roadmap" },
                ],
            },
        ],
        search: { provider: "local" },
        editLink: {
            pattern: "https://github.com/arief-fajri/pgbase/edit/main/docs/:path",
        },
        socialLinks: [{ icon: "github", link: "https://github.com/arief-fajri/pgbase" }],
        footer: {
            message: "MIT Licensed",
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
