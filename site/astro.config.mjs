// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	site: 'https://sageil.github.io',
	base: '/kodacode',
	integrations: [
		starlight({
			title: 'KodaCode',
			tagline: 'Truly yours.',
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/sageil/kodacode' }],
			components: {
				SocialIcons: './src/components/SocialIcons.astro',
			},
			customCss: [
				'@fontsource/inter/300.css',
				'@fontsource/inter/400.css',
				'@fontsource/inter/500.css',
				'@fontsource/inter/600.css',
				'@fontsource/inter/700.css',
				'@fontsource/inter/800.css',
				'./src/styles/custom.css',
			],
			sidebar: [
				{
					label: 'Getting Started',
					items: [
						{ label: 'Introduction', slug: 'getting-started/introduction' },
						{ label: 'Installation', slug: 'getting-started/installation' },
						{ label: 'Quick Start', slug: 'getting-started/quick-start' },
					],
				},
				{
					label: 'Features',
					items: [
						{ label: 'Sandbox', slug: 'features/sandbox' },
						{ label: 'Permissions', slug: 'features/permissions' },
						{ label: 'Multi-Provider AI', slug: 'features/providers' },
						{ label: 'Agent System', slug: 'features/agents' },
						{ label: 'Built-in Tools', slug: 'features/tools' },
						{ label: 'Project Memory', slug: 'features/memory' },
						{ label: 'Context Management', slug: 'features/context' },
						{ label: 'Sessions', slug: 'features/sessions' },
						{ label: 'Cost Tracking', slug: 'features/cost-tracking' },
						{ label: 'MCP Integration', slug: 'features/mcp' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'Configuration', slug: 'reference/configuration' },
						{ label: 'Slash Commands', slug: 'reference/commands' },
						{ label: 'Keyboard Shortcuts', slug: 'reference/shortcuts' },
						{ label: 'HTTP API', slug: 'reference/api' },
					],
				},
				{
					label: 'Architecture',
					items: [
						{ label: 'Overview', slug: 'architecture/overview' },
					],
				},
			],
		}),
	],
});
