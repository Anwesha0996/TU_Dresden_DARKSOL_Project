<?php

declare(strict_types=1);

$sourceRoot = getenv('APP_SOURCE_ROOT') ?: dirname(__DIR__) . '/src';
require_once $sourceRoot . '/ResultRepository.php';

function e(string $value): string
{
    return htmlspecialchars($value, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
}

$csvPath = getenv('RESULTS_CSV') ?: dirname(__DIR__, 2) . '/experiments/store-forward/results/persistent_store_forward_results.csv';
$repository = new ResultRepository($csvPath);
$allRows = $repository->all();
$filters = [
    'image_name' => trim((string) ($_GET['image'] ?? '')),
    'network_scenario' => trim((string) ($_GET['scenario'] ?? '')),
    'interruption_duration_seconds' => trim((string) ($_GET['outage'] ?? '')),
    'success' => trim((string) ($_GET['success'] ?? '')),
];
$rows = $repository->filter($allRows, $filters);
$summary = $repository->summary($rows);
$unique = static function (array $rows, string $column): array {
    $values = array_unique(array_filter(array_column($rows, $column), static fn(string $value): bool => $value !== ''));
    sort($values);
    return $values;
};
$rows = array_slice(array_reverse($rows), 0, 250);
?>
<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>DTN OCI Experiment Results</title>
    <link rel="stylesheet" href="/assets/style.css">
</head>
<body>
<header>
    <div><span class="eyebrow">tud-dtn-lifecycle-management</span><h1>DTN OCI image-transfer results</h1></div>
    <p>Per-run evidence for LEO, Lunar and Mars contact experiments.</p>
</header>
<main>
    <?php if ($allRows === []): ?>
        <section class="notice"><strong>No CSV data found.</strong> Mount the results file and set <code>RESULTS_CSV</code>.</section>
    <?php endif; ?>
    <form class="filters" method="get">
        <label>Image<select name="image"><option value="">All</option><?php foreach ($unique($allRows, 'image_name') as $value): ?><option <?= $filters['image_name'] === $value ? 'selected' : '' ?>><?= e($value) ?></option><?php endforeach; ?></select></label>
        <label>Scenario<select name="scenario"><option value="">All</option><?php foreach ($unique($allRows, 'network_scenario') as $value): ?><option <?= $filters['network_scenario'] === $value ? 'selected' : '' ?>><?= e($value) ?></option><?php endforeach; ?></select></label>
        <label>Interruption<select name="outage"><option value="">All</option><option value="10" <?= $filters['interruption_duration_seconds'] === '10' ? 'selected' : '' ?>>10 seconds</option><option value="300" <?= $filters['interruption_duration_seconds'] === '300' ? 'selected' : '' ?>>5 minutes</option><option value="3600" <?= $filters['interruption_duration_seconds'] === '3600' ? 'selected' : '' ?>>1 hour</option></select></label>
        <label>Result<select name="success"><option value="">All</option><option value="true" <?= $filters['success'] === 'true' ? 'selected' : '' ?>>Success</option><option value="false" <?= $filters['success'] === 'false' ? 'selected' : '' ?>>Failure</option></select></label>
        <button type="submit">Apply filters</button><a href="/">Reset</a>
    </form>
    <section class="cards">
        <article><span>Recorded attempts</span><strong><?= $summary['runs'] ?></strong></article>
        <article><span>Success rate</span><strong><?= number_format($summary['success_rate'], 1) ?>%</strong></article>
        <article><span>Failures</span><strong><?= $summary['failure'] ?></strong></article>
        <article><span>Digest matches</span><strong><?= $summary['digest_matches'] ?></strong></article>
        <article><span>Average duration</span><strong><?= number_format($summary['average_duration'], 1) ?> s</strong></article>
    </section>
    <section class="explanation">
        <strong>RTT note:</strong> LEO/Lunar values are configured network-emulation parameters. Mars is contact-only: 1800 s is a reference RTT, while applied propagation RTT is 0 ms.
    </section>
    <section class="table-wrap">
        <table>
            <thead><tr><th>Run</th><th>Image</th><th>Scenario</th><th>Outage</th><th>Payload</th><th>Duration</th><th>CPU S/R</th><th>RSS S/R</th><th>RTT / BW</th><th>Digest</th><th>Result</th><th>Error</th></tr></thead>
            <tbody>
            <?php foreach ($rows as $row): ?>
                <tr>
                    <td><code><?= e($row['run_id'] ?? '') ?></code></td><td><?= e($row['image_name'] ?? '') ?></td><td><?= e(strtoupper($row['network_scenario'] ?? '')) ?></td>
                    <td><?= e($row['interruption_duration_seconds'] ?? '') ?> s</td><td><?= number_format((float) ($row['payload_size_mb'] ?? 0), 2) ?> MB</td><td><?= number_format((float) ($row['duration_seconds'] ?? 0), 2) ?> s</td>
                    <td><?= e($row['sender_cpu_percent'] ?? '') ?> / <?= e($row['receiver_peak_cpu_percent'] ?? '') ?>%</td><td><?= number_format((float) ($row['sender_peak_memory_mb'] ?? 0), 1) ?> / <?= number_format((float) ($row['receiver_peak_memory_mb'] ?? 0), 1) ?> MB</td>
                    <td><?= e($row['configured_rtt'] ?? '') ?> / <?= e($row['configured_bandwidth'] ?? '') ?></td><td><span class="badge <?= ($row['digest_match'] ?? '') === 'true' ? 'ok' : 'bad' ?>"><?= e($row['digest_match'] ?? 'unknown') ?></span></td>
                    <td><span class="badge <?= ($row['success'] ?? '') === 'true' ? 'ok' : 'bad' ?>"><?= ($row['success'] ?? '') === 'true' ? 'PASS' : 'FAIL' ?></span></td><td class="error"><?= e($row['error_message'] ?? '') ?></td>
                </tr>
            <?php endforeach; ?>
            </tbody>
        </table>
    </section>
    <p class="limit">Showing the newest 250 filtered rows. The CSV remains the source of truth.</p>
</main>
</body>
</html>
