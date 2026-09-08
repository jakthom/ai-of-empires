import { test, expect } from './fixtures';

test('renders the interactive same-origin API reference', async ({ page }) => {
  const contract = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/openapi.json');
  await page.goto('/api/docs');
  expect((await contract).status()).toBe(200);
  await expect(page).toHaveTitle('AI of Empires API Reference');
  await expect(page.getByText('AI of Empires API', { exact: false }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'Check server health HTTP' })).toBeVisible();
});
