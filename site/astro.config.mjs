// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	site: 'https://kodacode.dev',
	integrations: [
		starlight({
			title: 'KodaCode',
			tagline: 'Truly yours.',
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/sageil/kodacode' }],
			components: {
				SocialIcons: './src/components/SocialIcons.astro',
			},
			disable404Route: true,
			customCss: [
				'@fontsource/inter/300.css',
				'@fontsource/inter/400.css',
				'@fontsource/inter/500.css',
				'@fontsource/inter/600.css',
				'@fontsource/inter/700.css',
				'@fontsource/inter/800.css',
				'@fontsource/jetbrains-mono/400.css',
				'@fontsource/jetbrains-mono/500.css',
				'@fontsource/jetbrains-mono/700.css',
				'./src/styles/custom.css',
				],
			sidebar: [
				{
					label: 'Getting Started',
					collapsed: true,
					items: [
						{ label: 'Introduction', slug: 'getting-started/introduction' },
						{ label: 'Installation', slug: 'getting-started/installation' },
						{ label: 'Quick Start', slug: 'getting-started/quick-start' },
						{ label: 'Common Workflows', slug: 'getting-started/workflows' },
					],
				},
				{
					label: 'Core Concepts',
					collapsed: true,
					items: [
						{ label: 'Sessions', slug: 'features/sessions' },
						{ label: 'Agents', slug: 'features/agents' },
						{ label: 'Project Memory & Instructions', slug: 'features/project-memory' },
						{ label: 'Tools', slug: 'features/tools' },
						{ label: 'Sandbox & Permissions', slug: 'features/sandbox' },
					],
				},
				{
					label: 'Models & Providers',
					collapsed: true,
					items: [
						{ label: 'Providers', slug: 'features/providers' },
						{ label: 'Model Routing', slug: 'features/model-routing' },
						{ label: 'Context Management', slug: 'features/context' },
						{ label: 'Budgets', slug: 'features/budgets' },
						{ label: 'Cost Tracking & Optimization', slug: 'features/cost-tracking' },
					],
				},
				{
					label: 'Extensions',
					collapsed: true,
					items: [
						{ label: 'MCP Servers', slug: 'features/mcp' },
						{ label: 'Skills', slug: 'features/skills' },
						{ label: 'Web Search', slug: 'features/web-search' },
						{ label: 'Search', slug: 'features/search' },
						{ label: 'Code Intelligence & LSP', slug: 'features/lsp' },
					],
				},
				{
					label: 'Reference',
					collapsed: true,
					items: [
						{ label: 'Configuration', slug: 'reference/configuration' },
						{ label: 'Slash Commands', slug: 'reference/commands' },
						{ label: 'Navigation Keys', slug: 'reference/navigation' },
						{ label: 'Keyboard Shortcuts', slug: 'reference/shortcuts' },
						{ label: 'TUI Layouts', slug: 'reference/layouts' },
					],
				},
				{
					label: 'Architecture',
					collapsed: true,
					items: [
						{ label: 'Overview', slug: 'architecture/overview' },
					],
				},
			],
		}),
	],
});
