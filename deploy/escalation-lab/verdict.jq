# Verdict of one run report: {"pass": bool, "failures": [...]}.
def sent(level; channel):
  level as $l | channel as $c | any(.deliveries[]; .level_index == $l and .channel_id == $c and .status == "sent");

def check(cond; msg): if cond then empty else msg end;

[
  check(.alert.alert_type == "container_down"; "alert is not container_down"),
  check(.alert.severity == "critical"; "alert is not critical"),
  check(.run.status == "exhausted"; "run ended \(.run.status), not exhausted"),
  check(.run.last_executed_level_index == 2; "last level executed is \(.run.last_executed_level_index), not 2"),
  check(sent(0; .channels.l1_webhook); "level 1 not sent to the webhook"),
  check(sent(1; .channels.l2_email); "level 2 not sent to the on-call email"),
  check(sent(2; .channels.l3_email); "level 3 not sent to the CTO email"),
  check(sent(2; .channels.l3_webhook); "level 3 not sent to the pager webhook"),
  check(sent(-3; .channels.l3_email); "exhaustion notice not sent to the CTO email"),
  check(sent(-3; .channels.l3_webhook); "exhaustion notice not sent to the pager webhook"),
  check(all(.deliveries[]; (.channel_name // "") != ""); "a delivery carries no channel_name"),
  check((.webhooks.l1 | length) >= 1; "the level 1 webhook received \(.webhooks.l1 | length) request(s), expected at least 1"),
  check((.webhooks.l3 | length) >= 2; "the level 3 webhook received \(.webhooks.l3 | length) request(s), expected at least 2"),
  check((.mails["oncall@lab.test"] // 0) >= 1; "oncall@lab.test received \(.mails["oncall@lab.test"] // 0) mail(s), expected at least 1"),
  check((.mails["cto@lab.test"] // 0) >= 2; "cto@lab.test received \(.mails["cto@lab.test"] // 0) mail(s), expected at least 2"),
  check(.healed == true; "the alert did not resolve after the restart"),
  check(.shop_web_state == "running"; "shop-web is \(.shop_web_state) after a SIGHUP it survives")
] | {pass: (length == 0), failures: .}
