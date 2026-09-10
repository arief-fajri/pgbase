import DefaultTheme from "vitepress/theme";
import MermaidViewer from "./MermaidViewer.vue";
import DocMeta from "./DocMeta.vue";
import "./custom.css";

export default {
    extends: DefaultTheme,
    enhanceApp({ app }) {
        app.component("MermaidViewer", MermaidViewer);
        app.component("DocMeta", DocMeta);
    },
};
