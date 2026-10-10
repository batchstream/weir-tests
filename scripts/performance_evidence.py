"""Validate the binary and source provenance shared by benchmark runs and summaries."""
import re


def require_digest(value, field, length=64):
    if not isinstance(value, str) or re.fullmatch('[0-9a-f]{' + str(length) + '}', value) is None:
        raise ValueError(field + ': missing or invalid immutable digest')


def validate_server_receipt(receipt, binary_sha256, versions):
    require_digest(binary_sha256, 'server_binary_sha256')
    require_digest(receipt.get('source'), 'server_receipt.source', 40)
    if receipt.get('sha256') != binary_sha256:
        raise ValueError('server receipt SHA does not match the server binary')
    empty = {}
    identity = receipt.get('identity', empty)
    if not isinstance(identity, dict) or not isinstance(identity.get('revision'), str) or not identity['revision']:
        raise ValueError('server receipt must record the binary identity revision')
    if 'source_tree_sha256' in receipt or receipt.get('source_kind') == 'frozen local Go source tree':
        require_digest(receipt.get('source_tree_sha256'), 'server_receipt.source_tree_sha256')
        require_digest(receipt.get('source_diff_sha256'), 'server_receipt.source_diff_sha256')
        if not isinstance(receipt.get('source_dirty'), bool):
            raise ValueError('local server receipt must record source_dirty as a boolean')
        return 'local'
    if receipt.get('version') != versions.get('weir_version') or not receipt.get('version'):
        raise ValueError('locked server receipt version does not match the pinned manifest')
    if receipt['source'] != versions.get('weir_source') or identity['revision'] != receipt['source']:
        raise ValueError('locked server receipt source or binary revision does not match the pinned manifest')
    return 'locked'


def validate_report_provenance(report, receipt):
    for field in ('client_source_tree_sha256', 'client_binary_sha256', 'server_binary_sha256'):
        require_digest(receipt.get(field), field)
    if receipt.get('validation_error'):
        raise ValueError('benchmark receipt records a validation failure: ' + receipt['validation_error'])
    for field in ('server_binary_before_sha256', 'server_binary_after_sha256'):
        if field in receipt and receipt[field] != receipt['server_binary_sha256']:
            raise ValueError(field + ': server binary changed during the matrix')
    empty = {}
    versions = receipt.get('versions', empty)
    server_receipt = receipt.get('server_receipt')
    if not isinstance(server_receipt, dict):
        raise ValueError('missing server receipt')
    kind = validate_server_receipt(server_receipt, receipt['server_binary_sha256'], versions)
    expected = dict(weir_binary_sha256=receipt['server_binary_sha256'], weir_source=server_receipt['source'])
    for field in ('sdk', 'protocol', 'mongodb_image', 'elasticsearch_image'):
        if not isinstance(versions.get(field), str) or not versions[field]:
            raise ValueError('missing pinned ' + field)
        expected[field] = versions[field]
    if kind == 'local':
        expected.update(weir_source_tree_sha256=server_receipt['source_tree_sha256'],
                        weir_source_diff_sha256=server_receipt['source_diff_sha256'],
                        weir_source_dirty=str(server_receipt['source_dirty']).lower(),
                        weir_binary_revision=server_receipt['identity']['revision'],
                        weir_source_kind='explicit local source build; unpublished changes may be present')
    else:
        expected['weir_source_kind'] = 'locked immutable module source'
    actual = report.get('provenance', empty)
    for field, value in expected.items():
        if actual.get(field) != value:
            raise ValueError('report provenance ' + field + ' does not match the recorded server receipt or pins')
