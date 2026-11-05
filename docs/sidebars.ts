import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

const appName = process.env.APP_NAME || "Waifu API";

let apiSidebarItems: object[] = [];
try {
  apiSidebarItems = require("./docs/api/sidebar");
} catch {
}

const sidebars: SidebarsConfig = {
  docs: [
    "intro",
    "getting-started",
    "authentication",
    "rate-limiting",
    "versioning",
    "tags",
    "terms-of-service",
  ],
  api: [
    {
      type: "category",
      label: "API Reference",
      link: {
        type: "generated-index",
        title: `${appName} API Reference`,
        description: `The complete API reference for ${appName}, auto-generated from the OpenAPI specification.`,
        slug: "/category/api",
      },
      items: ["api-intro", ...apiSidebarItems],
    },
  ],
};

export default sidebars;
