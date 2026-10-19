"""Validate the binary and source provenance shared by benchmark runs and summaries."""
import re

from server_source import METADATA, NESTED, production_file, safe_path, validate_identity


LOCAL_SOURCE = 'frozen local Go source tree'
FULL_GIT_SOURCE = 'verified full git checkout'


def require_digest(value, field, length=64):
    if not isinstance(value, str) or re.fullmatch('[0-9a-f]{' + str(length) + '}', value) is None:
        raise ValueError(field + ': missing or invalid immutable digest')


def validate_source_manifest(manifest):
    if not isinstance(manifest, list) or not manifest:
        raise ValueError('complete-source receipt must record source_files')
    paths = []
    nested = set()
    for entry in manifest:
        if not isinstance(entry, dict) or set(entry) != {'path', 'sha256'}:
            raise ValueError('invalid source_files manifest entry')
        name = entry['path']
        if not isinstance(name, str):
            raise ValueError('invalid source_files path')
        safe_path(name, name.startswith('third_party/grpc/'))
        if any(part in ('vendor', 'testdata') for part in name.split('/')):
            raise ValueError('source_files path is outside the server production allowlist: ' + name)
        require_digest(entry['sha256'], 'source_files.sha256')
        allowed = name in ('go.mod', 'go.sum', 'third_party/README.md')
        for prefix in ('cmd/', 'internal/', 'pkg/', 'api/'):
            allowed = allowed or (name.startswith(prefix) and production_file(name))
        for relative in NESTED.values():
            if name.startswith(relative + '/'):
                child = name[len(relative) + 1:]
                allowed = production_file(child) or child in METADATA
                nested.add(relative)
        if not allowed:
            raise ValueError('source_files path is outside the server production allowlist: ' + name)
        paths.append(name)
    if paths != sorted(set(paths)) or not {'go.mod', 'go.sum'}.issubset(paths):
        raise ValueError('source_files must be sorted, unique and include root module metadata')
    if not any(name.startswith('cmd/weir/') and production_file(name) for name in paths):
        raise ValueError('source_files is missing the server command')
    for relative in nested:
        required = {relative + '/' + name for name in
                    ('go.mod', 'LICENSE', 'WEIR_UPSTREAM.json', 'WEIR_PATCH.diff', 'WEIR_PATCH.md')}
        if not required.issubset(paths) or 'third_party/README.md' not in paths:
            raise ValueError('source_files is missing nested module provenance metadata')
    # Receipt JSON records these hashes; without source bytes it cannot re-prove their origin.


def identity_toolchain(identity):
    version, target = identity.get('go'), identity.get('target')
    if (not isinstance(version, str) or re.fullmatch(r'go[0-9]+\.[0-9]+(?:\.[0-9]+)?', version) is None
            or not isinstance(target, str) or re.fullmatch(r'[a-z0-9]+/[a-z0-9]+', target) is None):
        raise ValueError('server identity must record a Go toolchain and target')
    return version, target


def validate_server_receipt(receipt, binary_sha256, versions):
    if not isinstance(receipt, dict) or not isinstance(versions, dict):
        raise ValueError('server receipt and pinned manifest must be objects')
    require_digest(binary_sha256, 'server_binary_sha256')
    require_digest(receipt.get('source'), 'server_receipt.source', 40)
    if receipt.get('sha256') != binary_sha256:
        raise ValueError('server receipt SHA does not match the server binary')
    empty = {}
    identity = receipt.get('identity', empty)
    if not isinstance(identity, dict) or not isinstance(identity.get('revision'), str) or not identity['revision']:
        raise ValueError('server receipt must record the binary identity revision')
    kind = receipt.get('source_kind')
    if kind == LOCAL_SOURCE:
        require_digest(receipt.get('source_tree_sha256'), 'server_receipt.source_tree_sha256')
        require_digest(receipt.get('source_diff_sha256'), 'server_receipt.source_diff_sha256')
        if not isinstance(receipt.get('source_dirty'), bool):
            raise ValueError('local server receipt must record source_dirty as a boolean')
        if any(field in identity for field in ('product', 'version', 'state', 'dirty', 'go', 'target')):
            validate_identity(identity, identity_toolchain(identity))
        return 'local'
    if kind != FULL_GIT_SOURCE:
        # Old locked receipts remain readable for historical report audits only.
        provenance_fields = ('source_tree_sha256', 'source_files', 'source_diff_sha256', 'source_snapshot')
        if 'source_kind' in receipt or any(field in receipt for field in provenance_fields):
            raise ValueError('unknown or missing server receipt source_kind')
        if 'source_dirty' in receipt and receipt['source_dirty'] is not False:
            raise ValueError('locked server receipt must not be dirty')
    if receipt.get('version') != versions.get('weir_version') or not receipt.get('version'):
        raise ValueError('locked server receipt version does not match the pinned manifest')
    if receipt['source'] != versions.get('weir_source') or identity['revision'] != receipt['source']:
        raise ValueError('locked server receipt source or binary revision does not match the pinned manifest')
    if kind == FULL_GIT_SOURCE:
        if receipt.get('source_dirty') is not False:
            raise ValueError('complete-source locked receipt must record source_dirty=false')
        if 'source_diff_sha256' in receipt or 'source_snapshot' in receipt:
            raise ValueError('locked receipt must not contain local-only source provenance')
        if not versions.get('go') or receipt.get('go') != versions['go']:
            raise ValueError('locked server receipt Go version does not match the pinned manifest')
        toolchain = identity_toolchain(identity)
        if toolchain[0] != 'go' + versions['go']:
            raise ValueError('locked server identity Go version does not match the pinned manifest')
        validate_identity(identity, toolchain, receipt['source'])
        require_digest(receipt.get('source_tree_sha256'), 'server_receipt.source_tree_sha256')
        validate_source_manifest(receipt.get('source_files'))
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
