
ARG ROS_DOCKER=ros:melodic-ros-base

FROM golang:latest as golang_stage

FROM $ROS_DOCKER
COPY --from=golang_stage /usr/local/go /usr/local/go

RUN apt-get update && apt-get install -y wget vim

ENV GOPATH=/root
ENV PATH=$PATH:$GOPATH/bin:/usr/local/go/bin
RUN mkdir -p /root/src/github.com/fetchrobotics/rosgo
ADD . /root/src/github.com/fetchrobotics/rosgo
WORKDIR /root/src/github.com/fetchrobotics/rosgo
RUN cd gengo && go install
RUN cd ros && go build
ENTRYPOINT ["/ros_entrypoint.sh"]
CMD "bash"

