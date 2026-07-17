import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

sys.dont_write_bytecode = True
GA_ROOT = Path(__file__).resolve().parents[4]
if str(GA_ROOT) not in sys.path:
    sys.path.insert(0, str(GA_ROOT))
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

    def test_mapped_restore_uses_conversation_mode_and_returns_display_mapping(self):
        worker._bind_worldline_head(self.store, self.root, 'sid-1', {
            'node_id': 'b', 'turn_status': 'completed', 'has_final_answer': True,
            'user_message_id': 'u1', 'assistant_message_id': 'a1',
            'display_path': [2, 4],
        })
        emitted = []
        restore_result = {'history': [{'role': 'user', 'content': 'restored'}]}
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
                'action': 'restore_mapped', 'sid': 'sid-1', 'node_id': 'b'
            })
        restore.assert_called_once_with(self.store, 'b', mode='conversation', to='at')
        apply_restore.assert_called_once()
        self.assertEqual(emitted[0]['result']['display_path'], [2, 4])
        self.assertEqual(emitted[0]['result']['user_message_id'], 'u1')
        self.assertEqual(emitted[0]['result']['assistant_message_id'], 'a1')


if __name__ == '__main__':
    unittest.main()
