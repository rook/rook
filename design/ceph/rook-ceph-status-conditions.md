# Rook Ceph Operator Status Conditions

Reference: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md

## Background

Currently, Ceph Cluster stores the state of the system in `Status.State`. But, we want to implement the usage of `Status.Conditions` instead of using `Status.State`. The usage of `Status.Phase` is deprecated over time because it contradicts the system design principle and hampered evolution. So rather than encouraging clients to infer the implicit properties from phases, the usage of `Status.Condition` is preferred. Conditions are more extensible since the addition of new conditions doesn't invalidate decisions based on existing conditions, and are also better suited to reflect conditions that may toggle back and forth and/or that may not be mutually exclusive.

## Conditions

Conditions simply represent the latest available observation of an object's state. They are an extension mechanism intended to be used when the details of observation are not a priori known or would not apply to all instances of a given Kind. Objects can have multiple Conditions, and new types of Conditions can also be added in the future by the third-party controllers. Thus, Conditions are thereby represented using list/slice, where each having the similar structure.

## System States for rook-ceph

The necessary system states for the rook-ceph can be portrayed as follows:
	   
	   Ignored 		: If any of the resources gets ignored for multiple reasons
	   Progressing 		: Marks the start of reconcile of Ceph Cluster
	   Ready 		: When Reconcile completes successfully
	   Not Ready 		: Either when cluster is Updated or Updating is blocked
	   Connecting		: When the Ceph Cluster is in the state of Connecting 
	   Connected		: When the Ceph Cluster gets connected
	   Available 		: The Ceph Cluster is healthy and is ready to use
	   Failure 		: If any failure occurs in the Ceph Cluster
	   Cluster Expanding	: If the Cluster is Expanding
	   Upgrading		: When the Cluster gets an Upgrade

## Implementation Details

Reference: https://github.com/openshift/custom-resource-status/:

The `Status` of the Condition can be toggled between True or False according to the state of the cluster which it goes through. This can be shown to the user in the clusterCR with along with the information about the Conditions like the `Reason`, `Message` etc. Also a readable status, which basically states the final condition of the cluster along with the Message, which gives out some detail about the Condition like whether the Cluster is 'ReadytoUse' or if there is an Update available, we can update the MESSAGE as 'UpdateAvailable'. This could make it more understandable of the state of cluster to the user. Also, a Condition which states that the Cluster is undergoing an Upgrading can be added. Cluster Upgrade happens when there is a new version is available and changes the current Cluster CR. This will help the user to know the status of the Cluster Upgrade in progress. 
	
	   NAME        DATADIRHOSTPATH   MONCOUNT   AGE    CONDITION	MESSAGE     HEALTH
	   rook-ceph   /var/lib/rook     3          114s   Available	ReadyToUse  HEALTH_OK


We can add Conditions simply in the Custom Resource struct as:
	  
	   type ClusterStatus struct{
		FinalCondition ConditionType	  `json:"finalcondition,omitempty"`
		Message	       string		  `json:"message,omitmepty"`
		Condition      []RookConditions   `json:"conditions,omitempty"`
		CephStatus     *CephStatus        `json:"ceph,omitempty"`
	   }

After that we can just make changes inside rook ceph codebase as necessary. The `setStatusCondition()` field will be fed with the `newCondition` variable which holds the entries for the new Conditions. The `FindStatusCondition` will return the Condition if it is having the same `ConditionType` as the `newCondition` otherwise, it will return `nil`. If `nil` is returned then `LastHeartbeatTime` and `LastTransitionTime` is updated and gets appended to the `Condition`. The `Condition.Status` gets updated if the value is different from the `existingCondition.Status`. Rest of the fields of the `Status.Condition` are also updated. The `FinalCondition` will be holding the final condition the cluster is in. This will be displayed into the readable status along with a message, which is an extra useful information for the users.


The definition of the type Conditions can have the following details:
	   
	   Type               RookConditionType  `json:"type" description:"type of Rook condition"`
  	   Status             ConditionStatus    `json:"status" description:"status of the condition, one of True, False, Unknown"`
  	   Reason             *string            `json:"reason,omitempty" description:"one-word CamelCase reason for the condition's last transition"`
  	   Message            *string            `json:"message,omitempty" description:"human-readable message indicating details about last transition"`
	   LastHeartbeatTime  *unversioned.Time  `json:"lastHeartbeatTime,omitempty" description:"last time we got an update on a given condition"`
	   LastTransitionTime *unversioned.Time  `json:"lastTransitionTime,omitempty" description:"last time the condition transition from one status to another"`

The fields `Reason`, `Message`, `LastHeartbeatTime`, `LastTransitionTime` are optional field. Though the use of `Reason` field is encouraged.

Condition Types field specifies the current state of the system. Condition status values may be `True`, `False`, or `Unknown`. The absence of a condition should be interpreted the same as Unknown. How controllers handle Unknown depends on the Condition in question.
`Reason` is intended to be a one-word, CamelCase representation of the category of cause of the current status, and `Message` is intended to be a human-readable phrase or sentence, which may contain specific details of the individual occurrence. `Reason` is intended to be used in concise output, such as one-line kubectl get output, and in summarizing occurrences of causes, whereas `Message` is intended to be presented to users in detailed status explanations, such as `kubectl describe output`.

In the CephClusterStatus, we can either remove the `Status.State` and `Status.Message` fields and call the `Conditions` structure from inside the `ClusterStatus`, or we can just add the `Conditions` structure by keeping the already included fields.The first method is preferred because the `Conditions` structure contains the `Conditions.Type` and `Conditions.Message` which is similar to the `Status.State` and `Status.Message`. According to the above changes, necessary changes are to be made everywhere `ClusterStatus` or one of its fields are referred. 



### Examples

Consider a cluster is being created. The RookConditions is an array that can store multiple Conditions. So the progression of the cluster being created can be seen in the RookConditions as shown in the example below. The Ceph Cluster gets created after it establishes a successful Connection. The `RookCondition` will show in the slice that the `Connecting` Condition will be in `Condition.Status` False. The `Connected` and `Progressing` Types will be set to True.

	   Before:
		ClusterStatus{
		    State      :   Creating,
	   	    Message    :   The Cluster is getting created,
		}
	   After:
		ClusterStatus{
		     RookConditions{
			{
			  Type    :   Connecting,
			  Status  :   False,
			  Reason  :   ClusterConnecting,
			  Message :   The Cluster is Connecting,
			},
			{
			  Type    :   Connected,
			  Status  :   True,
			  Reason  :   ClusterConnected,
			  Message :   The Cluster is Connected,
			},
			{
			  Type    :   Progressing,
			  Status  :   True,
			  Reason  :   ClusterCreating,
			  Message :   The Cluster is getting created,
			},
		     },
		}
When a Cluster is getting updated, the `NotReady` Condition will be set to `True` and the `Ready` Condition will be set to `False`.


	   Before:
		ClusterStatus{
		    State      :   Updating,
	   	    Message    :   The Cluster is getting updated,
		}
	   After:
		ClusterStatus{
		     RookConditions{
			{
			  Type    :   Connecting,
			  Status  :   False,
			  Reason  :   ClusterConnecting,
			  Message :   The Cluster is Connecting,
			},
			{
			  Type    :   Connected,
			  Status  :   True,
			  Reason  :   ClusterConnected,
			  Message :   The Cluster is Connected,
			},
			{
			  Type    :   Progressing,
			  Status  :   False,
			  Reason  :   ClusterCreating,
			  Message :   The Cluster is getting created,
			},
			{
			  Type    :   Ready,
			  Status  :   False,
			  Reason  :   ClusterReady,
			  Message :   The Cluster is ready,
			},
			{
			  Type    :   Available,
			  Status  :   True,
			  Reason  :   ClusterAvailable,
		   	  Message :   The Cluster is healthy and available to use,
			},
			{
		          Type    :   NotReady,
			  Status  :   True,
			  Reason  :   ClusterUpdating,
 			  Message :   The Cluster is getting Updated,
			},
		     },
		}

In the examples mentioned above, the `LastTransitionTime` and `LastHeartbeatTime` is not added. These fields will also be included in the actual implementation and works in way such that when there is any change in the `Condition.Status` of a Condition, then the `LastTransitionTime` of that particular `Condition` will gets updated. For eg. in the second example indicated above, the `Condition.Status` of the `Condition` is shifted from `True` to `False` while cluster is Updating. So the `LastTranisitionTime` will gets updated when the shifting happens. `LastHeartbeatTime` gets updated whenever the `Condition` is getting updated.
