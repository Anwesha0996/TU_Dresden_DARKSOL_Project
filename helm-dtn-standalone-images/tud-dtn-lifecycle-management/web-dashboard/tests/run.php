<?php

declare(strict_types=1);

require_once dirname(__DIR__) . '/src/ResultRepository.php';

$path = tempnam(sys_get_temp_dir(), 'dtn-results-');
if ($path === false) {
    throw new RuntimeException('Cannot create temporary file');
}
file_put_contents($path, "run_id,image_name,network_scenario,duration_seconds,success,digest_match\nrun01,flannel,leo,10,true,true\nrun02,etcd,mars,20,false,false\n");
$repository = new ResultRepository($path);
$rows = $repository->all();
assert(count($rows) === 2);
assert(count($repository->filter($rows, ['network_scenario' => 'leo'])) === 1);
$summary = $repository->summary($rows);
assert($summary['runs'] === 2);
assert($summary['success'] === 1);
assert($summary['failure'] === 1);
assert($summary['success_rate'] === 50.0);
unlink($path);
echo "Dashboard tests passed\n";
