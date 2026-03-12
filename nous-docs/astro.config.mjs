import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://docs.nouschain.org',
  integrations: [
    starlight({
      title: 'NOUS',
      description: 'Documentation for the NOUS blockchain — Cogito Consensus',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/nous-reasoner/nous' },
        { icon: 'x.com', label: 'Twitter', href: 'https://x.com/nouschain' },
      ],
      customCss: ['./src/styles/custom.css'],
      sidebar: [
        {
          label: 'Overview',
          items: [
            { label: 'Introduction', slug: 'overview/introduction' },
            { label: 'How It Works', slug: 'overview/how-it-works' },
            { label: 'Tokenomics', slug: 'overview/tokenomics' },
          ],
        },
        {
          label: 'Getting Started',
          items: [
            { label: 'Download & Install', slug: 'getting-started/install' },
            { label: 'Quick Start', slug: 'getting-started/quickstart' },
            { label: 'Create a Wallet', slug: 'getting-started/wallet' },
          ],
        },
        {
          label: 'Reasoning (Mining)',
          items: [
            { label: 'ProbSAT Solver', slug: 'guides/reasoning/probsat' },
            { label: 'AI-Guided Mode', slug: 'guides/reasoning/ai-guided' },
            { label: 'Custom Solver', slug: 'guides/reasoning/custom-solver' },
          ],
        },
        {
          label: 'Wallet',
          items: [
            { label: 'Send & Receive', slug: 'guides/wallet/send-receive' },
            { label: 'Import Private Key', slug: 'guides/wallet/import-key' },
            { label: 'Backup & Recovery', slug: 'guides/wallet/backup' },
          ],
        },
        {
          label: 'Concepts',
          items: [
            { label: '3-SAT & Proof of Work', slug: 'concepts/3sat-pow' },
            { label: 'Difficulty Adjustment', slug: 'concepts/difficulty' },
            { label: 'Transactions & Fees', slug: 'concepts/transactions' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'RPC API', slug: 'reference/rpc-api' },
            { label: 'CLI Flags', slug: 'reference/cli' },
            { label: 'Whitepaper', slug: 'reference/whitepaper' },
          ],
        },
      ],
    }),
  ],
});
