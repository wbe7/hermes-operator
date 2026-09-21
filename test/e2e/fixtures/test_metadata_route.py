import unittest
from metadata_route import validate_node

class OwnershipGuardTests(unittest.TestCase):
    def node(self):return {'Id':'a'*64,'Name':'/hermes-operator-e2e-control-plane','Config':{'Labels':{'io.x-k8s.kind.cluster':'hermes-operator-e2e','io.x-k8s.kind.role':'control-plane'}},'HostConfig':{'NetworkMode':'kind'}}
    def test_own_namespace_only(self):self.assertEqual(validate_node(self.node()),'a'*64)
    def test_host_and_wrong_cluster_rejected(self):
        node=self.node();node['HostConfig']['NetworkMode']='host'
        with self.assertRaises(ValueError):validate_node(node)
        node=self.node();node['Config']['Labels']['io.x-k8s.kind.cluster']='production'
        with self.assertRaises(ValueError):validate_node(node)
        node=self.node();node['Name']='/other-control-plane'
        with self.assertRaises(ValueError):validate_node(node)

if __name__=='__main__':unittest.main()
