import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
  integrations: [
    starlight({
      title: 'gh-dash',
      description: 'A terminal dashboard for GitHub PRs, issues, and notifications.',
      social: {
        github: 'https://github.com/dlvhdr/gh-dash',
      },
      sidebar: [
        {
          label: 'Overview',
          items: [
            { label: 'Introduction', slug: 'overview/introduction' },
          ],
        },
        {
          label: 'Configuration',
          items: [
            { label: 'PR Sections', slug: 'configuration/pr-sections' },
            { label: 'Issue Sections', slug: 'configuration/issue-sections' },
            { label: 'Notification Sections', slug: 'configuration/notification-sections' },
            { label: 'Projects Sections', slug: 'configuration/projects-section' },
          ],
        },
        {
          label: 'Keybindings',
          items: [
            { label: 'Global Keybindings', slug: 'keybindings/global' },
            { label: 'Projects View', slug: 'keybindings/projects-view' },
          ],
        },
      ],
    }),
  ],
});
