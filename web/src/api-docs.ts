import { createApiReference } from '@scalar/api-reference';
import '@scalar/api-reference/style.css';

createApiReference(document.getElementById('api-reference')!, {
  url: '/api/v1/openapi.json',
  proxyUrl: '',
  layout: 'modern',
  theme: 'purple',
  darkMode: true,
  hideClientButton: true,
  hideDownloadButton: false,
  hideModels: false,
  metaData: {
    title: 'AI of Empires API Reference',
    description: 'Server-authoritative strategy game API.',
  },
  _integration: 'go',
});
