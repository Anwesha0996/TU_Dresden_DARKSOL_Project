<?php

declare(strict_types=1);

final class ResultRepository
{
    public function __construct(private readonly string $csvPath)
    {
    }

    /** @return list<array<string, string>> */
    public function all(): array
    {
        if (!is_readable($this->csvPath)) {
            return [];
        }

        $handle = fopen($this->csvPath, 'rb');
        if ($handle === false) {
            return [];
        }

        $headers = fgetcsv($handle);
        if ($headers === false) {
            fclose($handle);
            return [];
        }

        $rows = [];
        while (($values = fgetcsv($handle)) !== false) {
            if (count($values) !== count($headers)) {
                continue;
            }
            $row = array_combine($headers, $values);
            if ($row !== false) {
                $rows[] = $row;
            }
        }
        fclose($handle);

        return $rows;
    }

    /**
     * @param list<array<string, string>> $rows
     * @return list<array<string, string>>
     */
    public function filter(array $rows, array $filters): array
    {
        return array_values(array_filter($rows, static function (array $row) use ($filters): bool {
            foreach ($filters as $column => $value) {
                if ($value !== '' && ($row[$column] ?? '') !== $value) {
                    return false;
                }
            }
            return true;
        }));
    }

    /** @param list<array<string, string>> $rows */
    public function summary(array $rows): array
    {
        $success = count(array_filter($rows, static fn(array $row): bool => ($row['success'] ?? '') === 'true'));
        $digestMatches = count(array_filter($rows, static fn(array $row): bool => ($row['digest_match'] ?? '') === 'true'));
        $durations = array_map(
            static fn(array $row): float => (float) ($row['duration_seconds'] ?? 0),
            array_filter($rows, static fn(array $row): bool => is_numeric($row['duration_seconds'] ?? null))
        );

        return [
            'runs' => count($rows),
            'success' => $success,
            'failure' => count($rows) - $success,
            'success_rate' => count($rows) > 0 ? (float) (100 * $success / count($rows)) : 0.0,
            'digest_matches' => $digestMatches,
            'average_duration' => count($durations) > 0 ? array_sum($durations) / count($durations) : 0.0,
        ];
    }
}
