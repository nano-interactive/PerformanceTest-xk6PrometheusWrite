<?php

$rnd = 0;

parse_str($_SERVER["QUERY_STRING"], $params);
$rnd = @(int)$params['rnd'];
if($rnd > 0 && !empty($params["err"])) {
    if ($rnd > 95 && $rnd < 98) {
        http_response_code(404);
    } elseif ($rnd > 98) {
        http_response_code(500);
    }
}

echo("<h1>Works :)</h1>");
