import base64
import importlib.util
import json
import os
import sys
import tempfile
import unittest
import zlib
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest import mock

sys.dont_write_bytecode = True
RUNTIME_ROOT = Path(__file__).resolve().parent
if str(RUNTIME_ROOT) not in sys.path:
    sys.path.insert(0, str(RUNTIME_ROOT))
WORKER_PATH = Path(__file__).with_name('chat_worker.py')
SPEC = importlib.util.spec_from_file_location('ga_admin_chat_worker_worldline_tested', WORKER_PATH)
worker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(worker)


class FakeStore:
    def __init__(self):
        self.root_id = 'root'
        self.head = 'b'
        self.nodes = {
            'root': {'parent': None, 'children': ['a', 'b'], 'title': 'same'},
            'a': {'parent': 'root', 'children': [], 'title': 'same'},
            'b': {'parent': 'root', 'children': ['c'], 'title': 'same'},
            'c': {'parent': 'b', 'children': [], 'title': 'leaf'},
        }


class WorldlineSidecarTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.store = FakeStore()

    def tearDown(self):
        self.tmp.cleanup()

    def test_bundled_worldline_path_wins_over_ga_root(self):
        original_path = list(sys.path)
        worker_dir = Path(worker.__file__).resolve().parent
        try:
            sys.path[:] = [str(self.root / 'genericagent'), str(worker_dir)]
            worker._ensure_bundled_worldline_import_path()
            self.assertEqual(Path(sys.path[0]).resolve(), worker_dir)
        finally:
            sys.path[:] = original_path

    def test_packaged_worker_replaces_stale_worldline_sidecar(self):
        packaged_root = self.root / 'packaged' / 'cmd'
        packaged_frontends = packaged_root / 'frontends'
        packaged_frontends.mkdir(parents=True)
        worker_path = packaged_root / 'chat_worker.py'
        worker_path.write_text('# packaged worker\n', encoding='utf-8')
        target = packaged_frontends / 'worldline.py'
        target.write_text('from rich.console import Console\n', encoding='utf-8')
        bundled = b'from pathlib import Path\n'
        payload = base64.b64encode(zlib.compress(bundled)).decode('ascii')

        with mock.patch.object(worker, '__file__', str(worker_path)), \
             mock.patch.object(worker, '_BUNDLED_WORLDLINE_B64', payload):
            worker._ensure_bundled_worldline_runtime()

        self.assertEqual(target.read_bytes(), bundled)

    def test_worldline_hook_guard_is_defined_and_installs_once(self):
        registrations = []
        plugins = ModuleType('plugins')
        hooks = ModuleType('plugins.hooks')

        def register(event):
            self.assertEqual(event, 'tool_before')

            def decorator(callback):
                registrations.append(callback)
                return callback

            return decorator

        hooks.register = register
        plugins.hooks = hooks
        with mock.patch.object(worker, '_WORLDLINE_HOOK_INSTALLED', False), \
             mock.patch.dict(sys.modules, {'plugins': plugins, 'plugins.hooks': hooks}):
            worker._install_worldline_hook()
            worker._install_worldline_hook()
            self.assertTrue(worker._WORLDLINE_HOOK_INSTALLED)

        self.assertEqual(len(registrations), 1)

    def test_worldline_title_strips_project_mode_without_store_private_helper(self):
        store = SimpleNamespace(
            root_id='root',
            head='root',
            nodes={'root': {'parent': None, 'children': []}},
            rebuild_history=lambda _node_id: [],
        )
        history = [{
            'role': 'user',
            'content': (
                'Keep this title\n\n'
                '---\n[PROJECT MODE: ga-admin]\n'
                'Injected project instructions\n---\n'
            ),
        }]

        self.assertEqual(worker._worldline_title(store, history, 'fallback'), 'Keep this title')

    def test_worldline_title_extracts_text_from_structured_content(self):
        store = SimpleNamespace(
            root_id='root',
            head='root',
            nodes={'root': {'parent': None, 'children': []}},
            rebuild_history=lambda _node_id: [],
        )
        history = [{
            'role': 'user',
            'content': [{
                'type': 'text',
                'text': (
                    'Readable node title\n\n'
                    '---\n[PROJECT MODE: ga-admin]\n'
                    'Injected project instructions\n---\n'
                ),
            }],
        }]

        self.assertEqual(
            worker._worldline_title(store, history, 'fallback'),
            'Readable node title',
        )

    def test_worldline_title_skips_tool_result_messages_masquerading_as_user(self):
        store = SimpleNamespace(
            root_id='root',
            head='root',
            nodes={'root': {'parent': None, 'children': []}},
            rebuild_history=lambda _node_id: [],
        )
        history = [
            {
                'role': 'user',
                'content': [
                    {'type': 'tool_result', 'content': {'result': '{"status":"success"}'}},
                    {'type': 'text', 'text': '### [WORKING MEMORY]\ninternal state'},
                ],
            },
            {
                'role': 'user',
                'content': [{'type': 'text', 'text': 'Actual human request'}],
            },
        ]

        self.assertEqual(
            worker._worldline_title(store, history, 'fallback'),
            'Actual human request',
        )

    def test_worldline_title_falls_back_when_only_new_user_entry_is_a_tool_result(self):
        store = SimpleNamespace(
            root_id='root',
            head='root',
            nodes={'root': {'parent': None, 'children': []}},
            rebuild_history=lambda _node_id: [],
        )
        history = [{
            'role': 'user',
            'content': [{'type': 'tool_result', 'content': {'result': 'not a title'}}],
        }]

        self.assertEqual(worker._worldline_title(store, history, 'fallback prompt'), 'fallback prompt')

    def test_worldline_content_text_handles_nested_result(self):
        content = [{'type': 'tool_result', 'content': {'result': 'Useful result'}}]
        self.assertEqual(worker._worldline_content_text(content), 'Useful result')

    def test_imports_each_completed_turn_as_a_distinct_mapped_node(self):
        from frontends.worldline import RewindStore

        store = RewindStore(str(self.root / 'worldline'), str(self.root))
        backend = SimpleNamespace(history=[])
        agent = SimpleNamespace(
            llmclient=SimpleNamespace(backend=backend),
            history=[],
            handler=SimpleNamespace(working={}),
            _admin_worldline_store=store,
        )
        messages = []
        raw_history = []
        for number in range(1, 4):
            question = f'question {number}'
            answer = f'answer {number}'
            messages.extend([
                {'id': f'u-{number}', 'role': 'user', 'content': question},
                {'id': f'a-{number}', 'role': 'assistant', 'content': answer},
            ])
            raw_history.extend([
                {'role': 'user', 'content': [{'type': 'text', 'text': question}]},
                {'role': 'assistant', 'content': [{'type': 'text', 'text': answer}]},
            ])
        messages.append({'id': 'u-pending', 'role': 'user', 'content': 'pending question'})
        raw_history.append({'role': 'user', 'content': [{'type': 'text', 'text': 'pending question'}]})

        node_ids = worker._import_worldline_history(
            agent, store, self.root, 'sid-multi', messages, raw_history, [], {},
        )

        self.assertEqual(len(node_ids), 3)
        self.assertEqual(store.path_to(store.head), ['origin', *node_ids])
        self.assertEqual([store.nodes[node_id]['title'] for node_id in node_ids], [
            'question 1', 'question 2', 'question 3',
        ])
        self.assertEqual(len(store.rebuild_history(node_ids[0])), 2)
        self.assertEqual(len(store.rebuild_history(node_ids[1])), 4)
        self.assertEqual(len(store.rebuild_history(node_ids[2])), 6)
        self.assertEqual(len(agent.llmclient.backend.history), 7)

        sidecar, status = worker._load_worldline_sidecar(self.root, 'sid-multi')
        self.assertEqual(status, 'ok')
        self.assertEqual([sidecar['bindings'][node_id]['user_message_id'] for node_id in node_ids], [
            'u-1', 'u-2', 'u-3',
        ])
        self.assertEqual([len(sidecar['bindings'][node_id]['display_path']) for node_id in node_ids], [
            2, 4, 6,
        ])

    def test_repairs_a_persisted_multi_turn_aggregate_without_losing_history(self):
        from frontends.worldline import RewindStore

        store = RewindStore(str(self.root / 'legacy-worldline'), str(self.root))
        raw_history = [{'role': 'system', 'content': 'persisted system context'}]
        messages = []
        for number in range(1, 5):
            question = f'legacy question {number}'
            answer = f'legacy answer {number}'
            raw_history.extend([
                {'role': 'user', 'content': [{'type': 'text', 'text': question}]},
                {'role': 'assistant', 'content': [{'type': 'text', 'text': answer}]},
            ])
            messages.extend([
                {'id': f'legacy-u-{number}', 'role': 'user', 'content': question},
                {'id': f'legacy-a-{number}', 'role': 'assistant', 'content': answer},
            ])
        legacy_id = store.commit('legacy question 3', history=raw_history[:7])
        worker._bind_worldline_head(store, self.root, 'sid-legacy', {
            'node_id': legacy_id, 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'legacy-u-3', 'assistant_message_id': 'legacy-a-3',
            'display_path': messages[:6],
        })
        later_id = store.commit('legacy question 4', history=raw_history)
        worker._bind_worldline_head(store, self.root, 'sid-legacy', {
            'node_id': later_id, 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'legacy-u-4', 'assistant_message_id': 'legacy-a-4',
            'display_path': messages,
        })
        agent = SimpleNamespace(
            llmclient=SimpleNamespace(backend=SimpleNamespace(history=[])),
            history=[], handler=SimpleNamespace(working={}),
            _admin_worldline_store=store,
        )

        inserted = worker._import_worldline_history(
            agent, store, self.root, 'sid-legacy', messages, raw_history, [], {},
        )

        path = store.path_to(store.head)
        self.assertEqual(len(inserted), 2)
        self.assertEqual(path[-2:], [legacy_id, later_id])
        self.assertEqual(len(path), 5)
        self.assertEqual(store.rebuild_history(store.head), raw_history)
        self.assertEqual([store.nodes[node_id]['title'] for node_id in path[1:]], [
            'legacy question 1', 'legacy question 2', 'legacy question 3', 'legacy question 4',
        ])
        sidecar, status = worker._load_worldline_sidecar(self.root, 'sid-legacy')
        self.assertEqual(status, 'ok')
        self.assertEqual([sidecar['bindings'][node_id]['user_message_id'] for node_id in path[1:]], [
            'legacy-u-1', 'legacy-u-2', 'legacy-u-3', 'legacy-u-4',
        ])
        self.assertEqual(worker._import_worldline_history(
            agent, store, self.root, 'sid-legacy', messages, raw_history, [], {},
        ), [])
        self.assertEqual(store.path_to(store.head), path)

    def test_repairs_single_legacy_node_when_raw_turns_do_not_match_display(self):
        from frontends.worldline import RewindStore

        store = RewindStore(str(self.root / 'legacy-mismatch'), str(self.root))
        messages = []
        for number in range(1, 4):
            assistant = {'id': f'mismatch-a-{number}', 'role': 'assistant', 'content': f'answer {number}'}
            if number == 2:
                assistant.update(content='', outputs=['answer 2 from outputs'])
            messages.extend([
                {'id': f'mismatch-u-{number}', 'role': 'user', 'content': f'question {number}'},
                assistant,
            ])
        legacy_raw = [
            {'role': 'system', 'content': 'keep this context'},
            {'role': 'user', 'content': 'question 1'},
            {'role': 'assistant', 'content': 'answer 1'},
            {'role': 'user', 'content': 'question 3'},
            {'role': 'assistant', 'content': 'answer 3'},
        ]
        legacy_id = store.commit('question 3', history=legacy_raw)
        worker._bind_worldline_head(store, self.root, 'sid-mismatch', {
            'node_id': legacy_id, 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'mismatch-u-3', 'assistant_message_id': 'mismatch-a-3',
            'display_path': messages,
        })
        agent = SimpleNamespace(_admin_worldline_store=store)

        inserted = worker._import_worldline_history(
            agent, store, self.root, 'sid-mismatch', messages, legacy_raw, [], {},
        )

        path = store.path_to(store.head)
        rebuilt = store.rebuild_history(store.head)
        self.assertEqual(len(inserted), 2)
        self.assertEqual(path[-1], legacy_id)
        self.assertEqual([store.nodes[node_id]['title'] for node_id in path[1:]], [
            'question 1', 'question 2', 'question 3',
        ])
        self.assertEqual(rebuilt[0], legacy_raw[0])
        self.assertEqual(store._turn_sig(rebuilt), ['question 1', 'question 2', 'question 3'])
        sidecar, status = worker._load_worldline_sidecar(self.root, 'sid-mismatch')
        self.assertEqual(status, 'ok')
        self.assertEqual([sidecar['bindings'][node_id]['user_message_id'] for node_id in path[1:]], [
            'mismatch-u-1', 'mismatch-u-2', 'mismatch-u-3',
        ])

    def test_projection_preserves_sibling_order_repeated_title_identity_and_path(self):
        nodes = {
            key: SimpleNamespace(
                id=key, parent_id=value['parent'], children=list(value['children']),
                title=value['title'], kind='edit', files=[], ago=0, rw_tag=None,
            )
            for key, value in self.store.nodes.items()
        }
        tree = SimpleNamespace(root_id='root', nodes=nodes)
        sidecar = worker._empty_worldline_sidecar('sid-1')
        sidecar['bindings']['a'] = {
            'user_message_id': 'u-a', 'assistant_message_id': 'm-a', 'display_path': [0]
        }
        with mock.patch('frontends.worldline.tree_from_store', return_value=tree):
            projection = worker._worldline_nodes(self.store, sidecar, 'ok')
        self.assertEqual([n['id'] for n in projection['nodes']], ['root', 'b', 'a', 'c'])
        self.assertEqual([n['title'] for n in projection['nodes'][:3]], ['same', 'same', 'same'])
        self.assertEqual(projection['current_path'], ['root', 'b'])
        by_id = {node['id']: node for node in projection['nodes']}
        self.assertEqual(by_id['a']['mapping_status'], 'mapped')
        self.assertEqual(by_id['b']['mapping_status'], 'unmapped')
        self.assertIsNone(by_id['b']['user_message_id'])

    def test_projection_collapses_internal_bridge_alias_and_reparents_descendant(self):
        store = SimpleNamespace(root_id='root', head='new', nodes={
            'root': {'parent': None, 'children': ['a', 'b', 'bridge']},
            'a': {'parent': 'root', 'children': []},
            'b': {'parent': 'root', 'children': []},
            'bridge': {'parent': 'root', 'children': ['new']},
            'new': {'parent': 'bridge', 'children': []},
        })
        nodes = {
            node_id: SimpleNamespace(
                id=node_id, parent_id=value['parent'], children=list(value['children']),
                title=node_id, kind='edit', files=[], ago=0, rw_tag=None,
            )
            for node_id, value in store.nodes.items()
        }
        sidecar = worker._empty_worldline_sidecar('sid-1')
        sidecar['aliases']['bridge'] = 'a'
        with mock.patch('frontends.worldline.tree_from_store', return_value=SimpleNamespace(root_id='root', nodes=nodes)):
            projection = worker._worldline_nodes(store, sidecar, 'ok')
        by_id = {node['id']: node for node in projection['nodes']}
        self.assertNotIn('bridge', by_id)
        self.assertEqual(projection['head'], 'new')
        self.assertEqual(projection['current_path'], ['root', 'a', 'new'])
        self.assertEqual(by_id['new']['parent_id'], 'a')
        self.assertIn('new', by_id['a']['children'])

    def test_projection_caps_oversized_topology_and_keeps_current_path(self):
        child_ids = [f'n-{index}' for index in range(worker._WORLDLINE_PUBLIC_NODE_LIMIT + 25)]
        store = SimpleNamespace(root_id='root', head=child_ids[-1], nodes={
            'root': {'parent': None, 'children': child_ids},
            **{node_id: {'parent': 'root', 'children': []} for node_id in child_ids},
        })
        nodes = {
            'root': SimpleNamespace(id='root', parent_id=None, children=child_ids, title='root', kind='edit', files=[], ago=0, rw_tag=None),
            **{node_id: SimpleNamespace(id=node_id, parent_id='root', children=[], title=node_id, kind='edit', files=[], ago=0, rw_tag=None) for node_id in child_ids},
        }
        with mock.patch('frontends.worldline.tree_from_store', return_value=SimpleNamespace(root_id='root', nodes=nodes)):
            projection = worker._worldline_nodes(store)
        ids = {node['id'] for node in projection['nodes']}
        self.assertEqual(projection['schema_version'], 1)
        self.assertEqual(len(projection['nodes']), worker._WORLDLINE_PUBLIC_NODE_LIMIT)
        self.assertTrue(projection['truncated'])
        self.assertEqual(projection['current_path'], ['root', child_ids[-1]])
        self.assertEqual(projection['head'], child_ids[-1])
        self.assertTrue(all(set(node['children']) <= ids for node in projection['nodes']))

    def test_state_source_nodes_keep_folded_physical_ancestors_private(self):
        store = SimpleNamespace(
            head='leaf',
            nodes={
                'root': {'parent': None, 'children': ['folded']},
                'folded': {'parent': 'root', 'children': ['leaf']},
                'leaf': {'parent': 'folded', 'children': []},
            },
        )
        sidecar = {'status': 'ok', 'bindings': {
            'folded': {'user_message_id': 'u-folded'},
            'leaf': {'user_message_id': 'u-leaf'},
            'sibling': {'user_message_id': 'u-sibling'},
        }}
        self.assertEqual(worker._worldline_source_nodes(store, sidecar), {
            'u-folded': 'folded',
            'u-leaf': 'leaf',
        })
        self.assertEqual(
            worker._worldline_rpc_result(store, sidecar, 'ok', 'state', None),
            {'source_nodes': {'u-folded': 'folded', 'u-leaf': 'leaf'}},
        )
        self.assertIsNone(worker._worldline_rpc_result(store, sidecar, 'ok', 'list', None))

    def test_completed_head_binding_is_atomic_and_persists_across_reload(self):
        req = {
            'node_id': 'b', 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'user-1', 'assistant_message_id': 'assistant-1',
            'display_path': ['root', 'b'],
        }
        result = worker._bind_worldline_head(self.store, self.root, 'sid-1', req)
        self.assertEqual(result['assistant_message_id'], 'assistant-1')
        loaded, status = worker._load_worldline_sidecar(self.root, 'sid-1')
        self.assertEqual(status, 'ok')
        self.assertEqual(loaded['bindings']['b']['display_path'], ['root', 'b'])
        ordinal = loaded['bindings']['b']['ordinal']
        created_at = loaded['bindings']['b']['created_at']
        self.assertGreaterEqual(ordinal, 1)
        self.assertGreater(created_at, 0)
        self.assertGreater(loaded['next_ordinal'], ordinal)
        rebound = dict(req, assistant_message_id='assistant-2')
        worker._bind_worldline_head(self.store, self.root, 'sid-1', rebound)
        reloaded, status = worker._load_worldline_sidecar(self.root, 'sid-1')
        self.assertEqual(status, 'ok')
        self.assertEqual(reloaded['bindings']['b']['ordinal'], ordinal)
        self.assertEqual(reloaded['bindings']['b']['created_at'], created_at)
        self.assertEqual(reloaded['bindings']['b']['assistant_message_id'], 'assistant-2')
        path = worker._worldline_sidecar_path(self.root, 'sid-1')
        self.assertEqual(json.loads(path.read_text(encoding='utf-8'))['schema_version'], 1)
        self.assertEqual(list(path.parent.glob('*.tmp-*')), [])

    def test_missing_malformed_legacy_and_sid_isolation_degrade_safely(self):
        missing, status = worker._load_worldline_sidecar(self.root, 'sid-a')
        self.assertEqual((status, missing['bindings']), ('missing', {}))
        path_a = worker._worldline_sidecar_path(self.root, 'sid-a')
        path_a.parent.mkdir(parents=True)
        path_a.write_text('{bad', encoding='utf-8')
        malformed, status = worker._load_worldline_sidecar(self.root, 'sid-a')
        self.assertEqual((status, malformed['bindings']), ('malformed', {}))
        path_a.write_text(json.dumps({'schema_version': 0, 'bindings': {'b': {}}}), encoding='utf-8')
        legacy, status = worker._load_worldline_sidecar(self.root, 'sid-a')
        self.assertEqual((status, legacy['bindings']), ('legacy', {}))
        req = {
            'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'u', 'assistant_message_id': 'a',
        }
        worker._bind_worldline_head(self.store, self.root, 'sid-b', req)
        isolated, status = worker._load_worldline_sidecar(self.root, 'sid-a')
        other, other_status = worker._load_worldline_sidecar(self.root, 'sid-b')
        self.assertEqual((status, isolated['bindings']), ('legacy', {}))
        self.assertEqual(other_status, 'ok')
        self.assertIn('b', other['bindings'])
        for bad_sid in ('', '../escape', 'a/b', 'a\\b'):
            with self.assertRaises(ValueError):
                worker._worldline_sidecar_path(self.root, bad_sid)

    def test_non_completed_or_non_final_turn_never_binds(self):
        base = {'user_message_id': 'u', 'assistant_message_id': 'a'}
        for req in (
            dict(base, turn_status='running', has_final_answer=True),
            dict(base, turn_status='completed', has_final_answer=False),
            dict(base, turn_status='completed'),
        ):
            with self.assertRaises(ValueError):
                worker._bind_worldline_head(self.store, self.root, 'sid-1', req)
        self.assertFalse(worker._worldline_sidecar_path(self.root, 'sid-1').exists())

    def test_only_current_head_can_be_bound(self):
        req = {
            'node_id': 'a', 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'u', 'assistant_message_id': 'a',
        }
        with self.assertRaises(ValueError):
            worker._bind_worldline_head(self.store, self.root, 'sid-1', req)

    def test_worldline_store_is_initialized_only_by_an_activating_request(self):
        emitted = []
        empty_store = SimpleNamespace(nodes={})
        history = [
            {'id': 'u-old', 'role': 'user', 'content': 'old question'},
            {'id': 'a-old', 'role': 'assistant', 'content': 'old answer'},
        ]
        agent = object()
        with mock.patch.object(worker, '_resolve_request_root', return_value=self.root), \
             mock.patch.object(worker, '_apply_workspace', return_value=None), \
             mock.patch.object(worker, '_ensure_worldline_store', return_value=empty_store) as ensure, \
             mock.patch.object(worker, '_import_worldline_history') as import_history, \
             mock.patch.object(worker, '_worldline_nodes', return_value={'nodes': []}), \
             mock.patch.object(worker, '_snapshot_backend_history', return_value=[]), \
             mock.patch.object(worker, '_snapshot_ga_state', return_value={}), \
             mock.patch.object(worker, 'emit', side_effect=emitted.append):
            worker.handle_worldline_request(agent, {
                'action': 'state', 'sid': 'sid-1',
            })
            ensure.assert_not_called()
            import_history.assert_not_called()
            self.assertEqual(emitted[-1]['tree']['sidecar_status'], 'inactive')

            worker.handle_worldline_request(agent, {
                'activate': True, 'action': 'list', 'sid': 'sid-1',
                'history': history,
                'raw_history': [{'role': 'assistant', 'content': 'backend'}],
                'history_info': [{'step': 1}],
                'working': {'key_info': 'restored'},
            })
            ensure.assert_called_once_with(agent, self.root, None)
            import_history.assert_called_once_with(
                agent, empty_store, self.root, 'sid-1', history,
                [{'role': 'assistant', 'content': 'backend'}],
                [{'step': 1}], {'key_info': 'restored'},
            )
            self.assertEqual(emitted[-1]['tree'], {'nodes': []})

    def test_mapped_restore_uses_core_conv_mode_and_returns_display_mapping(self):
        worker._bind_worldline_head(self.store, self.root, 'sid-1', {
            'node_id': 'b', 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'u1', 'assistant_message_id': 'a1',
            'display_path': [2, 4],
        })
        emitted = []
        restore_result = {'history': [{'role': 'user', 'content': 'restored'}], 'target': 'bridge-mapped'}
        with mock.patch.object(worker, '_resolve_request_root', return_value=self.root), \
             mock.patch.object(worker, '_apply_workspace', return_value=None), \
             mock.patch.object(worker, '_ensure_worldline_store', return_value=self.store), \
             mock.patch.object(worker, '_apply_worldline_restore') as apply_restore, \
             mock.patch.object(worker, '_worldline_nodes', return_value={'nodes': []}), \
             mock.patch.object(worker, '_snapshot_backend_history', return_value=[]), \
             mock.patch.object(worker, '_snapshot_ga_state', return_value={}), \
             mock.patch.object(worker, 'emit', side_effect=emitted.append), \
             mock.patch('frontends.worldline.restore_plan', return_value=restore_result) as restore:
            worker.handle_worldline_request(object(), {
                'activate': True, 'action': 'restore_mapped', 'sid': 'sid-1', 'node_id': 'b'
            })
        restore.assert_called_once_with(self.store, 'b', mode='conv', to='at')
        apply_restore.assert_called_once()
        self.assertEqual(emitted[0]['result']['display_path'], [2, 4])
        self.assertEqual(emitted[0]['result']['user_message_id'], 'u1')
        self.assertEqual(emitted[0]['result']['assistant_message_id'], 'a1')
        loaded, status = worker._load_worldline_sidecar(self.root, 'sid-1')
        self.assertEqual(status, 'ok')
        self.assertEqual(loaded['aliases']['bridge-mapped'], 'b')

    def test_public_conversation_restore_maps_to_core_conv_mode(self):
        emitted = []
        restore_result = {'history': [{'role': 'user', 'content': 'restored'}], 'target': 'bridge-before'}
        with mock.patch.object(worker, '_resolve_request_root', return_value=self.root), \
             mock.patch.object(worker, '_apply_workspace', return_value=None), \
             mock.patch.object(worker, '_ensure_worldline_store', return_value=self.store), \
             mock.patch.object(worker, '_apply_worldline_restore') as apply_restore, \
             mock.patch.object(worker, '_worldline_nodes', return_value={'nodes': []}), \
             mock.patch.object(worker, '_snapshot_backend_history', return_value=[]), \
             mock.patch.object(worker, '_snapshot_ga_state', return_value={}), \
             mock.patch.object(worker, 'emit', side_effect=emitted.append), \
             mock.patch('frontends.worldline.restore_plan', return_value=restore_result) as restore:
            worker.handle_worldline_request(object(), {
                'activate': True, 'action': 'restore', 'sid': 'sid-1', 'node_id': 'b',
                'mode': 'conversation', 'to': 'before',
            })
        restore.assert_called_once_with(self.store, 'b', mode='conv', to='before')
        apply_restore.assert_called_once_with(mock.ANY, restore_result)
        self.assertEqual(emitted[0]['result'], restore_result)
        loaded, status = worker._load_worldline_sidecar(self.root, 'sid-1')
        self.assertEqual(status, 'ok')
        self.assertEqual(loaded['aliases']['bridge-before'], 'root')

    def test_real_store_multi_hop_switch_commit_and_before_restore_stays_logical(self):
        from frontends.worldline import RewindStore

        workspace = self.root / 'workspace'
        workspace.mkdir()
        store = RewindStore(str(self.root / 'rewind'), str(workspace))
        history = []

        def commit_turn(title, user, assistant):
            history.extend([
                {'role': 'user', 'content': user},
                {'role': 'assistant', 'content': assistant},
            ])
            return store.commit(title, history=list(history))

        v1 = commit_turn('one', 'one', 'answer one')
        worker._bind_worldline_head(store, self.root, 'sid-real', {
            'node_id': v1, 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'u1', 'assistant_message_id': 'a1',
            'display_path': [0],
        })
        v2 = commit_turn('two', 'two', 'answer two')
        worker._bind_worldline_head(store, self.root, 'sid-real', {
            'node_id': v2, 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'u2', 'assistant_message_id': 'a2',
            'display_path': [0, 1],
        })
        commit_turn('three', 'three', 'answer three')

        def request(payload):
            emitted = []
            with mock.patch.object(worker, '_resolve_request_root', return_value=self.root), \
                 mock.patch.object(worker, '_apply_workspace', return_value=workspace), \
                 mock.patch.object(worker, '_ensure_worldline_store', return_value=store), \
                 mock.patch.object(worker, '_apply_worldline_restore'), \
                 mock.patch.object(worker, '_snapshot_backend_history', return_value=[]), \
                 mock.patch.object(worker, '_snapshot_ga_state', return_value={}), \
                 mock.patch.object(worker, 'emit', side_effect=emitted.append):
                worker.handle_worldline_request(object(), dict(payload, activate=True))
            self.assertEqual(len(emitted), 1)
            return emitted[0]

        first_switch = request({
            'action': 'restore_mapped', 'sid': 'sid-real', 'node_id': v1,
        })
        bridge1 = first_switch['result']['target']
        self.assertNotEqual(bridge1, v1)
        self.assertEqual(store.head, bridge1)
        child1_history = store.rebuild_history(store.head) + [
            {'role': 'user', 'content': 'one child'},
            {'role': 'assistant', 'content': 'answer one child'},
        ]
        child1 = store.commit('one child', history=child1_history)

        second_switch = request({
            'action': 'restore_mapped', 'sid': 'sid-real', 'node_id': v2,
        })
        bridge2 = second_switch['result']['target']
        self.assertNotEqual(bridge2, v2)
        child2_history = store.rebuild_history(store.head) + [
            {'role': 'user', 'content': 'two child'},
            {'role': 'assistant', 'content': 'answer two child'},
        ]
        child2 = store.commit('two child', history=child2_history)
        self.assertEqual(store.nodes[child2]['parent'], bridge2)

        before = request({
            'action': 'restore', 'sid': 'sid-real', 'node_id': child2,
            'mode': 'conversation', 'to': 'before',
        })
        bridge3 = before['result']['target']
        projection = before['tree']
        by_id = {node['id']: node for node in projection['nodes']}

        self.assertEqual(store.head, bridge3)
        self.assertEqual(projection['head'], v2)
        self.assertEqual(projection['current_path'][-2:], [v1, v2])
        self.assertTrue({bridge1, bridge2, bridge3}.isdisjoint(by_id))
        self.assertEqual(by_id[child1]['parent_id'], v1)
        self.assertEqual(by_id[child2]['parent_id'], v2)
        self.assertIn(child1, by_id[v1]['children'])
        self.assertIn(child2, by_id[v2]['children'])

        loaded, status = worker._load_worldline_sidecar(self.root, 'sid-real')
        self.assertEqual(status, 'ok')
        self.assertEqual(loaded['aliases'][bridge1], v1)
        self.assertEqual(loaded['aliases'][bridge2], v2)
        self.assertEqual(loaded['aliases'][bridge3], v2)

    def test_real_store_restores_admin_history_and_key_info(self):
        from frontends.worldline import RewindStore, restore_plan

        workspace = self.root / 'workspace-state'
        workspace.mkdir()
        store = RewindStore(str(self.root / 'rewind-state'), str(workspace))
        history = [
            {'role': 'user', 'content': 'stateful turn'},
            {'role': 'assistant', 'content': 'stateful answer'},
        ]
        hist_info = [{'role': 'assistant', 'summary': 'saved summary'}]
        node = store.commit('stateful', history=history, hist_info=hist_info, key_info='saved key')

        result = restore_plan(store, node, mode='conv', to='at')

        self.assertEqual(result['history'], history)
        self.assertEqual(result['hist_info'], hist_info)
        self.assertEqual(result['key_info'], 'saved key')


if __name__ == '__main__':
    unittest.main()
