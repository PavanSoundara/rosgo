#!/usr/bin/env python
import rospy
from std_msgs.msg import String


def talker():
    pub = rospy.Publisher('bratter', String)
    rospy.Subscriber('blatter', String)
    rospy.Subscriber('bratter', String)
    rospy.init_node('talker2')
    while not rospy.is_shutdown():
        str = "%s: hello world %s" % (rospy.get_name(), rospy.get_time())
        rospy.loginfo(str)
        pub.publish(String(str))
        rospy.sleep(1.0)


if __name__ == '__main__':
    try:
        talker()
    except rospy.ROSInterruptException:
        pass
