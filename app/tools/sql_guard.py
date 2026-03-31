from __future__ import annotations

import re


class SQLGuard:
    allowed_objects = {
        'v_refund_metrics_daily',
        'v_ticket_sla',
        'v_ticket_detail',
        'v_recent_releases',
    }

    forbidden_patterns = [
        r'\binsert\b',
        r'\bupdate\b',
        r'\bdelete\b',
        r'\bdrop\b',
        r'\balter\b',
        r'\btruncate\b',
        r'\bcreate\b',
        r'\bunion\b',
        r'\boutfile\b',
        r'\binformation_schema\b',
        r'\bmysql\b',
        r'\bsleep\s*\(',
        r'\bbenchmark\s*\(',
        r'--',
        r'/\*',
        r'\*/',
        r'#',
        r';',
    ]

    def validate(self, sql: str) -> dict:
        candidate = sql.strip()
        lowered = candidate.lower()
        if not lowered.startswith('select'):
            return {'passed': False, 'severity': 'error', 'message': 'SQL 只允许 SELECT'}
        for pattern in self.forbidden_patterns:
            if re.search(pattern, lowered):
                return {'passed': False, 'severity': 'error', 'message': f'SQL 含有禁用内容: {pattern}'}

        limit_match = re.search(r'\blimit\s+(\d+)\b', lowered)
        if not limit_match:
            return {'passed': False, 'severity': 'error', 'message': 'SQL 必须带 LIMIT'}
        if int(limit_match.group(1)) > 200:
            return {'passed': False, 'severity': 'error', 'message': 'LIMIT 最大为 200'}

        objects = self._extract_objects(candidate)
        if not objects:
            return {'passed': False, 'severity': 'error', 'message': 'SQL 未识别到合法查询对象'}
        disallowed = [item for item in objects if item not in self.allowed_objects]
        if disallowed:
            return {'passed': False, 'severity': 'error', 'message': f'SQL 访问了非白名单对象: {", ".join(disallowed)}'}
        return {'passed': True, 'severity': 'info', 'message': 'SQL 校验通过'}

    def _extract_objects(self, sql: str) -> set[str]:
        matches = re.findall(
            r'\b(?:from|join)\s+([`"\[]?[a-zA-Z_][\w$]*(?:[`"\]]?\.[`"\[]?[a-zA-Z_][\w$]*[`"\]]?)?)',
            sql,
            flags=re.IGNORECASE,
        )
        objects: set[str] = set()
        for raw in matches:
            cleaned = raw.replace('`', '').replace('"', '').replace('[', '').replace(']', '')
            objects.add(cleaned.split('.')[-1])
        return objects
