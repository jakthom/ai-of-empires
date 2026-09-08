import { test, expect } from './fixtures';

for (const [id, name] of [['easy', 'Easy'], ['extra_hard', 'Extra hard'], ['expert', 'Expert'], ['aggressive', 'Aggressive']]) {
  test(`starts ${name} from the catalog and retains its difficulty after reload`, async ({ page, game }, info) => {
    await game.start('skirmish', id);
    const snapshot = await game.snapshot();
    expect(snapshot.difficulty.id).toBe(id);
    await expect(page.locator('#match-difficulty')).toHaveText(name);
    await expect(page.locator('#match-difficulty')).toHaveAttribute('title', snapshot.difficulty.description);
    await page.reload();
    await expect(page.locator('#match-difficulty')).toHaveText(name);
    await page.getByRole('button', { name: 'Match menu', exact: true }).click();
    await page.getByRole('button', { name: 'Start a new match', exact: true }).click();
    await expect(page.locator('#difficulty option')).toHaveCount(7);
    await page.getByRole('combobox', { name: 'Difficulty', exact: true }).selectOption(id);
    await expect(page.locator('#difficulty-description')).toHaveText(snapshot.difficulty.description);
    await page.screenshot({ path: info.outputPath('difficulty-options.png') });
  });
}
