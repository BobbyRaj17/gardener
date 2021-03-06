// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package seedmanager_test

import (
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/garden"
	gardeninformers "github.com/gardener/gardener/pkg/client/garden/informers/internalversion"
	. "github.com/gardener/gardener/plugin/pkg/shoot/seedmanager"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/admission"

	seedmanagerapi "github.com/gardener/gardener/plugin/pkg/shoot/seedmanager/apis/seedmanager"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("seedmanager", func() {
	Describe("#Admit", func() {
		var (
			admissionHandler      *SeedManager
			gardenInformerFactory gardeninformers.SharedInformerFactory
			seed                  garden.Seed
			shoot                 garden.Shoot

			cloudProfileName = "cloudprofile-1"
			seedName         = "seed-1"
			region           = "europe"

			falseVar = false
			trueVar  = true

			seedBase = garden.Seed{
				ObjectMeta: metav1.ObjectMeta{
					Name: seedName,
				},
				Spec: garden.SeedSpec{
					Cloud: garden.SeedCloud{
						Profile: cloudProfileName,
						Region:  region,
					},
					Visible:   &trueVar,
					Protected: &falseVar,
					Networks: garden.SeedNetworks{
						Nodes:    gardencore.CIDR("10.10.0.0/16"),
						Pods:     gardencore.CIDR("10.20.0.0/16"),
						Services: gardencore.CIDR("10.30.0.0/16"),
					},
				},
				Status: garden.SeedStatus{
					Conditions: []gardencore.Condition{
						{
							Type:   garden.SeedAvailable,
							Status: gardencore.ConditionTrue,
						},
					},
				},
			}
			shootBase = garden.Shoot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shoot",
					Namespace: "my-namespace",
				},
				Spec: garden.ShootSpec{
					Cloud: garden.Cloud{
						Profile: cloudProfileName,
						Region:  region,
						AWS: &garden.AWSCloud{
							Networks: garden.AWSNetworks{
								K8SNetworks: gardencore.K8SNetworks{
									Nodes:    makeCIDRPtr("10.40.0.0/16"),
									Pods:     makeCIDRPtr("10.50.0.0/16"),
									Services: makeCIDRPtr("10.60.0.0/16"),
								},
							},
						},
					},
				},
			}

			defaultAdmissionConfiguration = seedmanagerapi.Configuration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "seedmanager.admission.config.gardener.cloud/v1alpha1",
					Kind:       "Configuration",
				},
				Strategy: seedmanagerapi.SameRegion,
			}
		)

		Context("Shoot references a Seed - protection", func() {
			BeforeEach(func() {
				admissionHandler, _ = New(&defaultAdmissionConfiguration)
				admissionHandler.AssignReadyFunc(func() bool { return true })
				gardenInformerFactory = gardeninformers.NewSharedInformerFactory(nil, 0)
				admissionHandler.SetInternalGardenInformerFactory(gardenInformerFactory)

				seed = *seedBase.DeepCopy()
				shoot = *shootBase.DeepCopy()

				shoot.Spec.Cloud.Seed = &seedName
			})

			It("should pass because the Seed specified in shoot manifest is not protected and shoot is not in garden namespace", func() {
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass because shoot is not in garden namespace and seed is not protected", func() {
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail because shoot is not in garden namespace and seed is protected", func() {
				seed.Spec.Protected = &trueVar

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})

			It("should pass because shoot is in garden namespace and seed is protected", func() {
				shoot.Namespace = "garden"
				seed.Spec.Protected = &trueVar

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass because shoot is in garden namespace and seed is not protected", func() {
				shoot.Namespace = "garden"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail because the networks of the shoot overlaps with the seed networks", func() {
				shoot.Spec.Cloud.AWS.Networks.K8SNetworks = gardencore.K8SNetworks{
					Pods:     &seed.Spec.Networks.Pods,
					Services: &seed.Spec.Networks.Services,
					Nodes:    &seed.Spec.Networks.Nodes,
				}
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
			})
		})

		Context("Shoot does not reference a Seed - find an adequate one using 'Same Region' seed determination strategy", func() {
			BeforeEach(func() {
				seedmanagerAdmission := defaultAdmissionConfiguration
				seedmanagerAdmission.Strategy = seedmanagerapi.SameRegion
				admissionHandler, _ = New(&seedmanagerAdmission)

				admissionHandler.AssignReadyFunc(func() bool { return true })
				gardenInformerFactory = gardeninformers.NewSharedInformerFactory(nil, 0)
				admissionHandler.SetInternalGardenInformerFactory(gardenInformerFactory)

				seed = *seedBase.DeepCopy()
				shoot = *shootBase.DeepCopy()

				shoot.Spec.Cloud.Seed = nil
			})

			It("should find a seed cluster 1) 'Same Region' seed determination strategy 2) referencing the same profile 3) same  region 4) indicating availability", func() {
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(seedName))
			})

			It("should find the best seed cluster 1) 'Same Region' seed determination strategy 2) referencing the same profile 3) same  region 4) indicating availability", func() {
				secondSeed := seedBase
				secondSeed.Name = "seed-2"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&secondSeed)

				secondShoot := shootBase
				secondShoot.Name = "shoot-2"
				// first seed references more shoots then seed-2 -> expect seed-2 to be selected
				secondShoot.Spec.Cloud.Seed = &seed.Name

				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(&secondShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(secondSeed.Name))
			})

			It("should fail because it cannot find a seed cluster  1) 'Same Region' seed determination strategy 2) region that no seed supports", func() {
				shoot.Spec.Cloud.Region = "another-region"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
				Expect(shoot.Spec.Cloud.Seed).To(BeNil())
			})
		})

		Context("Shoot does not reference a Seed - find an adequate one using 'Minimal Distance' seed determination strategy", func() {
			BeforeEach(func() {
				seedmanagerAdmission := defaultAdmissionConfiguration
				seedmanagerAdmission.Strategy = seedmanagerapi.MinimalDistance
				admissionHandler, _ = New(&seedmanagerAdmission)

				admissionHandler.AssignReadyFunc(func() bool { return true })
				gardenInformerFactory = gardeninformers.NewSharedInformerFactory(nil, 0)
				admissionHandler.SetInternalGardenInformerFactory(gardenInformerFactory)

				seed = *seedBase.DeepCopy()
				shoot = *shootBase.DeepCopy()

				shoot.Spec.Cloud.Seed = nil
			})

			It("should find a seed cluster 1) referencing the same profile 2) same  region 3) indicating availability", func() {
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(seedName))
			})

			It("should find a seed cluster  1) referencing the same profile 2) different region 3) indicating availability 4) only one seed existing", func() {
				anotherRegion := "another-region"
				shoot.Spec.Cloud.Region = anotherRegion

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(seedName))
				// verify that shoot is in another region than the seed
				Expect(shoot.Spec.Cloud.Region).NotTo(Equal(seed.Spec.Cloud.Region))
			})

			It("should find the seed cluster with the minimal distance 1) referencing the same profile 2) different region 3) indicating availability 4) multiple seeds existing", func() {
				// add 3 seeds with different names and regions
				seed.Spec.Cloud.Region = "europe-north1"

				secondSeed := seedBase
				secondSeed.Name = "seed-2"
				secondSeed.Spec.Cloud.Region = "europe-west1"

				thirdSeed := seedBase
				thirdSeed.Name = "seed-3"
				thirdSeed.Spec.Cloud.Region = "asia-south1"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&secondSeed)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&thirdSeed)

				// define shoot to be lexicographically 'closer' to the second seed
				anotherRegion := "europe-west3"
				shoot.Spec.Cloud.Region = anotherRegion

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(secondSeed.Name))
				// verify that shoot is in another region than the chosen seed
				Expect(shoot.Spec.Cloud.Region).NotTo(Equal(secondSeed.Spec.Cloud.Region))
			})

			It("should find the best seed cluster 1) referencing the same profile 2) same  region 3) indicating availability 4) multiple seeds existing", func() {
				secondSeed := seedBase
				secondSeed.Name = "seed-2"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&secondSeed)

				secondShoot := shootBase
				secondShoot.Name = "shoot-2"
				// first seed references more shoots then seed-2 -> expect seed-2 to be selected
				secondShoot.Spec.Cloud.Seed = &seed.Name

				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(&secondShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(secondSeed.Name))
			})
		})

		Context("Shoot does not reference a Seed - find an adequate one using default seed determination strategy", func() {
			BeforeEach(func() {
				admissionHandler, _ = New(&defaultAdmissionConfiguration)
				admissionHandler.AssignReadyFunc(func() bool { return true })
				gardenInformerFactory = gardeninformers.NewSharedInformerFactory(nil, 0)
				admissionHandler.SetInternalGardenInformerFactory(gardenInformerFactory)

				seed = *seedBase.DeepCopy()
				shoot = *shootBase.DeepCopy()

				shoot.Spec.Cloud.Seed = nil
			})

			It("should find a seed cluster 1) referencing the same profile 2) same  region 3) indicating availability", func() {
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(seedName))
			})

			It("should find the best seed cluster 1) referencing the same profile 2) same  region 3) indicating availability", func() {
				secondSeed := seedBase
				secondSeed.Name = "seed-2"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&secondSeed)

				secondShoot := shootBase
				secondShoot.Name = "shoot-2"
				secondShoot.Spec.Cloud.Seed = &seed.Name

				gardenInformerFactory.Garden().InternalVersion().Shoots().Informer().GetStore().Add(&secondShoot)

				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(*shoot.Spec.Cloud.Seed).To(Equal(secondSeed.Name))
			})

			It("should fail because it cannot find a seed cluster due to network disjointedness", func() {
				shoot.Spec.Cloud.AWS.Networks.K8SNetworks = gardencore.K8SNetworks{
					Pods:     &seed.Spec.Networks.Pods,
					Services: &seed.Spec.Networks.Services,
					Nodes:    &seed.Spec.Networks.Nodes,
				}

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
				Expect(shoot.Spec.Cloud.Seed).To(BeNil())
			})

			It("should fail because it cannot find a seed cluster due to region that no seed supports", func() {
				shoot.Spec.Cloud.Region = "another-region"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
				Expect(shoot.Spec.Cloud.Seed).To(BeNil())
			})

			It("should fail because it cannot find a seed cluster due to invalid profile", func() {
				shoot.Spec.Cloud.Profile = "another-profile"

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
				Expect(shoot.Spec.Cloud.Seed).To(BeNil())
			})

			It("should fail because it cannot find a seed cluster due to unavailability", func() {
				seed.Status.Conditions = []gardencore.Condition{
					{
						Type:   garden.SeedAvailable,
						Status: gardencore.ConditionFalse,
					},
				}

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
				Expect(shoot.Spec.Cloud.Seed).To(BeNil())
			})

			It("should fail because it cannot find a seed cluster due to invisibility", func() {
				seed.Spec.Visible = &falseVar

				gardenInformerFactory.Garden().InternalVersion().Seeds().Informer().GetStore().Add(&seed)
				attrs := admission.NewAttributesRecord(&shoot, nil, garden.Kind("Shoot").WithVersion("version"), shoot.Namespace, shoot.Name, garden.Resource("shoots").WithVersion("version"), "", admission.Create, false, nil)

				err := admissionHandler.Admit(attrs, nil)

				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsForbidden(err)).To(BeTrue())
				Expect(shoot.Spec.Cloud.Seed).To(BeNil())
			})
		})
	})
})

func makeCIDRPtr(cidr string) *gardencore.CIDR {
	c := gardencore.CIDR(cidr)
	return &c
}
